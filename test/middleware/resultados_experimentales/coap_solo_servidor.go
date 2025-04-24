package main

import (
	"bytes"
	"context"
	"log"
	"sync"
	"sync/atomic"

	coap "github.com/plgd-dev/go-coap/v3"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/mux"
)

// datos de las conexiones de los observadores
type Conexion struct {
	conexion mux.Conn
	context context.Context
	token []byte
}

// Almacena observadores por ruta
var (
	valor atomic.Int64        // valor de observación
	observadores []Conexion   // conexiones CoAP
	mutexCoAP sync.Mutex      // Mutex para proteger el acceso a `clientesPorTopico`
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

	// obtengo si tiene observe
	obs, err := r.Options().Observe()

	// Responder según el método
	switch {
	case metodo == codes.GET && err == nil && obs == 0:
		manejarSuscripcionCoAP(w, r)
	case metodo == codes.POST:
		mensaje, err := r.Message.ReadBody()
		if err != nil {
			log.Println("Error al procesar el cuerpo de la solicitud: "+ err.Error())
			return
		}
		manejarPublicacionCoAP(w, r, mensaje)
	default:
		log.Printf("Método no soportado: %v", metodo)
		err := w.SetResponse(codes.MethodNotAllowed, message.TextPlain, bytes.NewReader([]byte("Método no soportado")))
		if err != nil {
			log.Printf("Error al enviar respuesta: %v", err)
		}
	}
}

// manejarSuscripcionCoAP maneja las solicitudes GET con observe
func manejarSuscripcionCoAP (w mux.ResponseWriter, r *mux.Message) {

	// agrego observadores
	log.Println("Agregando observador")
	mutexCoAP.Lock()
	datosConexion := Conexion{w.Conn(), r.Context(), r.Token()}
	observadores = append(observadores, datosConexion)
	mutexCoAP.Unlock()

	// enviar respuesta
	err :=  enviarRespuesta(w.Conn(), r.Token(), nil, valor.Add(1))
	if err != nil {
		log.Printf("Error en transmitir: %v", err)
	}
}

// manejarPublicacionCoAP envía una publicación a los observadores
func manejarPublicacionCoAP (w mux.ResponseWriter, r *mux.Message, payload []byte) {

	err := w.SetResponse(codes.Created, message.TextPlain,  nil)
	if err != nil {
		log.Printf("Error al enviar respuesta: %v", err)
	}		

	// notifico a todos los observadores
	mutexCoAP.Lock()
	for _, o := range observadores {
		enviarRespuesta(o.conexion, o.token, payload, valor.Add(1))
	}
	mutexCoAP.Unlock()
}

func enviarRespuesta(cc mux.Conn, token []byte, mensaje []byte, obs int64) error {
	m := cc.AcquireMessage(cc.Context())
	defer cc.ReleaseMessage(m)
	m.SetCode(codes.Content)
	m.SetToken(token)
	m.SetBody(bytes.NewReader(mensaje))
	m.SetContentFormat(message.TextPlain)
	if obs >= 0 {
		m.SetObserve(uint32(obs))
	}
	return cc.WriteMessage(m)
}
