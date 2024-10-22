package main

import (
    "fmt"
    "log"
    "net/http"
)

func manejador(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hola mundo!")
}

func main() {
    http.HandleFunc("/", manejador)
    port := "9000"
    fmt.Printf("Iniciando servidor en puerto %s\n", port)
    if err := http.ListenAndServe(":"+port, nil); err != nil {
        log.Fatalf("No se ha podido iniciar el servidor: %s\n", err)
    }
}