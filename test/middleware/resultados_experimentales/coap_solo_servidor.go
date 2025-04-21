package main

import (
	"bytes"
	"log"

	coap "github.com/plgd-dev/go-coap/v3"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/mux"
)

func main() {
	r := mux.NewRouter()
	// Manejador para cualquier ruta
	r.Handle("/test", mux.HandlerFunc(manejadorCoAP))

	// Iniciar el servidor en el puerto especificado
	err := coap.ListenAndServe("udp", ":5683", r)
	if err != nil {
		log.Fatalf("Error al iniciar el servidor: %v", err)
	}
}

// handleAll maneja todas las solicitudes CoAP, independientemente de la ruta
func manejadorCoAP(w mux.ResponseWriter, r *mux.Message) {
	metodo := r.Code()

	// Responder según el método
	switch {
	case metodo == codes.GET:
		err := w.SetResponse(codes.Empty, message.TextPlain, bytes.NewReader([]byte("")))
		if err != nil {
			log.Printf("Error al enviar respuesta: %v", err)
		}
	case metodo == codes.POST:
		err := w.SetResponse(codes.Created, message.TextPlain, bytes.NewReader([]byte("")))
		if err != nil {
			log.Printf("Error al enviar respuesta: %v", err)
		}
	default:
		log.Printf("Método no soportado: %v", metodo)
		err := w.SetResponse(codes.MethodNotAllowed, message.TextPlain, bytes.NewReader([]byte("Método no soportado")))
		if err != nil {
			log.Printf("Error al enviar respuesta: %v", err)
		}
	}
}
