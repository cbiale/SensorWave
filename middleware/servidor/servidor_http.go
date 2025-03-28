package servidor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type Cliente struct {
    Channel chan string // Canal para enviar mensajes al cliente
}

// Clientes por topico (revisar el formato de unidad)
var (
    clientesPorTopico = make(map[string]map[*Cliente]bool) // Mapa de clientes agrupados por tópico
    mutexHTTP           sync.Mutex                         // Mutex para proteger el acceso a `clientesPorTopico`
)


const LOG_HTTP string = "HTTP"

// IniciarHTTP inicia un servidor HTTP en el puerto especificado
func IniciarHTTP (puerto string) {
    // Endpoint para manejar conexiones
    http.HandleFunc("/sensorwave", manejadorHTTP)

    loggerPrint(LOG_HTTP, "Iniciando servidor HTTP en :" + puerto)
    // loggerFatal(LOG, 
    http.ListenAndServe(":" + puerto, nil)
    //)
}

// manejador es el punto de entrada para todas las solicitudes HTTP
func manejadorHTTP(w http.ResponseWriter, r *http.Request) {
    loggerPrint(LOG_HTTP, "Solicitud " + r.Method + r.URL.Path)
    if r.Method == http.MethodGet {
        manejarSuscripcionHTTP (w, r)
    }
    if r.Method == http.MethodPost {
        manejarPublicacionHTTP (w, r)
    }
}

// Manejar conexiones SSE (ver de no pasar por URL)
func manejarSuscripcionHTTP (w http.ResponseWriter, r *http.Request) {
    // Obtener el tópico del cliente desde los parámetros de la URL
    topico := r.URL.Query().Get("topico")
    if topico == "" {
        http.Error(w, "Falta el parámetro 'topico'", http.StatusBadRequest)
        return
    }

    // Configurar los encabezados para SSE
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Crear un cliente y agregarlo al mapa de clientes por tópico
    cliente := &Cliente{Channel: make(chan string)}
    mutexHTTP.Lock()
    if _, exists := clientesPorTopico[topico]; !exists {
        clientesPorTopico[topico] = make(map[*Cliente]bool)
    }
    clientesPorTopico[topico][cliente] = true
    mutexHTTP.Unlock()

    // Flusher para enviar datos inmediatamente
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "El servidor no soporta streaming", http.StatusInternalServerError)
        return
    }

    loggerPrint(LOG_HTTP, "Cliente conectado al tópico " + topico)

    // Enviar mensajes al cliente en un bucle
    for msg := range cliente.Channel {
        fmt.Fprintf(w, "data: %s\n\n", msg)
        flusher.Flush()
    }

    // Limpiar la conexión cuando el cliente se desconecte
    mutexHTTP.Lock()
    delete(clientesPorTopico[topico], cliente)
    if len(clientesPorTopico[topico]) == 0 {
        delete(clientesPorTopico, topico) // Eliminar el tópico si no tiene clientes
    }
    mutexHTTP.Unlock()

    loggerPrint(LOG_HTTP, "Cliente desconectado del tópico " + topico)
}

// Manejar publicaciones de mensajes
func manejarPublicacionHTTP (w http.ResponseWriter, r *http.Request) {
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

	loggerPrint(LOG_HTTP, "Mensaje recibido en el tópico " + mensaje.Topico)

    // Enviar el mensaje a todos los clientes suscritos al tópico
    mutexHTTP.Lock()
    if clientes, exists := clientesPorTopico[mensaje.Topico]; exists {
        for cliente := range clientes {
            select {
            case cliente.Channel <- string(mensaje.Payload):
                loggerPrint(LOG_HTTP, "Mensaje enviado al tópico " + mensaje.Topico + ": " + string(mensaje.Payload))
            default:
                loggerPrint(LOG_HTTP, "No se pudo enviar el mensaje al cliente en el tópico " + mensaje.Topico + " (canal bloqueado)")
            }
        }
    } else {
        loggerPrint(LOG_HTTP, "No hay clientes suscritos al tópico " + mensaje.Topico)
    }
    mutexHTTP.Unlock()

    // enviar a los otros protocolos
    mensaje.Original = false
    enviarCoAP(LOG_HTTP, mensaje)
    enviarMQTT(LOG_HTTP, mensaje)
    // Responder al cliente que envió el POST
    w.WriteHeader(http.StatusOK)
}

// ver si es posible
func manejarDesuscripcionHTTP (w http.ResponseWriter, r *http.Request) {

}