package main

import (
    "log"
    "net/http"
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
    if r.Method != http.MethodGet || r.Method != http.MethodPost {
        http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
        return
    }

    // Responder al cliente
    w.WriteHeader(http.StatusOK)
    log.Println("Mensaje recibido")
}

