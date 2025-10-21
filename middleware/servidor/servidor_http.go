package servidor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Cliente struct {
	ID      string
	Channel chan string
	cerrado bool
	mu      sync.Mutex
}

var (
	clientesPorTopico = make(map[string]map[string]*Cliente)
	mutexHTTP         sync.Mutex
)

const LOG_HTTP string = "HTTP"

// IniciarHTTP inicia un servidor HTTP en el puerto especificado
func IniciarHTTP(puerto string) {
	// Endpoint para manejar conexiones
	http.HandleFunc("/sensorwave", manejadorHTTP)

	loggerPrint(LOG_HTTP, "Iniciando servidor HTTP en :"+puerto)
	http.ListenAndServe(":"+puerto, nil) // ver de poner loggerFatal
}

// manejador es el punto de entrada para todas las solicitudes HTTP
func manejadorHTTP(w http.ResponseWriter, r *http.Request) {
	loggerPrint(LOG_HTTP, "Solicitud "+r.Method+r.URL.Path)
	if r.Method == http.MethodGet {
		manejarSuscripcionHTTP(w, r)
	}
	if r.Method == http.MethodPost {
		manejarPublicacionHTTP(w, r)
	}
	if r.Method == http.MethodDelete {
		manejarDesuscripcionHTTP(w, r)
	}
}

func manejarSuscripcionHTTP(w http.ResponseWriter, r *http.Request) {
	topico := r.URL.Query().Get("topico")
	if topico == "" {
		http.Error(w, "Falta el parámetro 'topico'", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clienteID := fmt.Sprintf("%d", time.Now().UnixNano())
	cliente := &Cliente{
		ID:      clienteID,
		Channel: make(chan string, 10000),
		cerrado: false,
	}

	mutexHTTP.Lock()
	if clientesPorTopico[topico] == nil {
		clientesPorTopico[topico] = make(map[string]*Cliente)
	}
	clientesPorTopico[topico][clienteID] = cliente
	mutexHTTP.Unlock()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "El servidor no soporta streaming", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "data: {\"clienteID\":\"%s\"}\n\n", clienteID)
	flusher.Flush()

	loggerPrint(LOG_HTTP, "Cliente "+clienteID+" conectado al tópico "+topico)

	for msg := range cliente.Channel {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}

	mutexHTTP.Lock()
	if clientes, exists := clientesPorTopico[topico]; exists {
		delete(clientes, clienteID)
		if len(clientes) == 0 {
			delete(clientesPorTopico, topico)
		}
	}
	mutexHTTP.Unlock()

	cliente.mu.Lock()
	if !cliente.cerrado {
		close(cliente.Channel)
		cliente.cerrado = true
	}
	cliente.mu.Unlock()

	loggerPrint(LOG_HTTP, "Cliente "+clienteID+" desconectado del tópico "+topico)
}

// Manejar publicaciones de mensajes
func manejarPublicacionHTTP(w http.ResponseWriter, r *http.Request) {
	// Leer el cuerpo de la solicitud
	var mensaje Mensaje
	err := json.NewDecoder(r.Body).Decode(&mensaje)
	if err != nil {
		http.Error(w, "Error al procesar el cuerpo de la solicitud: "+err.Error(), http.StatusBadRequest)
		return
	}

	if mensaje.Topico == "" {
		http.Error(w, "Falta el parámetro 'topico'", http.StatusBadRequest)
		return
	}

	loggerPrint(LOG_HTTP, "Mensaje recibido en el tópico "+mensaje.Topico)

	// enviar a los protocolos
	if mensaje.Original {
		mensaje.Original = false
		go enviarHTTP(LOG_HTTP, mensaje)
		go enviarCoAP(LOG_HTTP, mensaje)
		go enviarMQTT(LOG_HTTP, mensaje)
		go enviarNATS(LOG_HTTP, mensaje)
	}
	// Responder al cliente que envió el POST
	w.WriteHeader(http.StatusOK)
}

func manejarDesuscripcionHTTP(w http.ResponseWriter, r *http.Request) {
	topico := r.URL.Query().Get("topico")
	clienteID := r.URL.Query().Get("clienteID")

	if topico == "" || clienteID == "" {
		http.Error(w, "Faltan parámetros 'topico' o 'clienteID'", http.StatusBadRequest)
		return
	}

	mutexHTTP.Lock()
	var clienteEncontrado *Cliente
	if clientes, exists := clientesPorTopico[topico]; exists {
		if cliente, existe := clientes[clienteID]; existe {
			clienteEncontrado = cliente
			delete(clientes, clienteID)
			if len(clientes) == 0 {
				delete(clientesPorTopico, topico)
			}
		}
	}
	mutexHTTP.Unlock()

	if clienteEncontrado != nil {
		clienteEncontrado.mu.Lock()
		if !clienteEncontrado.cerrado {
			close(clienteEncontrado.Channel)
			clienteEncontrado.cerrado = true
		}
		clienteEncontrado.mu.Unlock()
		loggerPrint(LOG_HTTP, "Cliente "+clienteID+" desuscrito del tópico "+topico)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Cliente no encontrado", http.StatusNotFound)
	}
}
