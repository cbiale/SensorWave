package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
)

type Cliente struct {
    Channel chan string // Canal para enviar mensajes al cliente
}

var (
    // slice
    clientes []*Cliente  // slice de clientes
    mutexHTTP sync.Mutex // Mutex para proteger el acceso a `clientes`
)

func main() {
    // Configurar el servidor HTTP
    http.HandleFunc("/test", manejadorHTTP)

    // Iniciar el servidor en el puerto 8080
    log.Println("Servidor HTTP escuchando en http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func manejadorHTTP(w http.ResponseWriter, r *http.Request) {
    // Acepto solo Gets o Posts
    if r.Method != http.MethodGet && r.Method != http.MethodPost {
        http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
        return
    }
    if r.Method == http.MethodGet {
        manejarSuscripcionHTTP(w, r)
    } 
    if r.Method == http.MethodPost {
        manejarPublicacionHTTP(w, r)
    }
    log.Println("Mensaje recibido")
}

// Manejar conexiones SSE (ver de no pasar por URL)
func manejarSuscripcionHTTP (w http.ResponseWriter, r *http.Request) {

    // Configurar los encabezados para SSE
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Crear un cliente y agregarlo al mapa de clientes
    cliente := &Cliente{Channel: make(chan string)}
    mutexHTTP.Lock()
	clientes = append(clientes, cliente)
    mutexHTTP.Unlock()
    

    // Flusher para enviar datos inmediatamente
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "El servidor no soporta streaming", http.StatusInternalServerError)
        return
    }

    log.Println("Cliente conectado")

    // Enviar mensajes al cliente en un bucle
    for msg := range cliente.Channel {
        fmt.Fprintf(w, "data: %s\n\n", msg)
        flusher.Flush()
    }

    // Limpiar la conexión cuando el cliente se desconecte
    mutexHTTP.Lock()
    // Eliminar el cliente del mapa de clientes por tópico
    for i, c := range clientes {
        if c == cliente {
            clientes = append(clientes[:i], clientes[i+1:]...) // Eliminar el cliente
            break
        }
    }
    // Cerrar el canal del cliente
    close(cliente.Channel)
    mutexHTTP.Unlock()

    log.Println("Cliente desconectado")
}

// Manejar publicaciones de mensajes
func manejarPublicacionHTTP (w http.ResponseWriter, r *http.Request) {
    // Leer el payload del requerimiento
    payload, err := io.ReadAll(r.Body)

    if err != nil {
        http.Error(w, "Error al leer el cuerpo de la solicitud", http.StatusBadRequest)
        return
    }

    mutexHTTP.Lock()
    for _, cliente := range clientes {
        select {
        case cliente.Channel <- string(payload):
            fmt.Println("Mensaje enviado al tópico")
        default:
            fmt.Println("No se pudo enviar el mensaje al cliente")
        }
    }
    mutexHTTP.Unlock()	

    w.WriteHeader(http.StatusOK)
}

