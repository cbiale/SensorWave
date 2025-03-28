package servidor

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	coap "github.com/plgd-dev/go-coap/v3"
	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/codes"
	"github.com/plgd-dev/go-coap/v3/mux"
)

const LOG_COAP = "COAP"

// datos de las conexiones de los observadores
type Conexion struct {
	conexion mux.Conn
	context context.Context
	token []byte
}

// Almacena observadores por ruta
var (
	valor atomic.Int64 						  // valor de observación
	observadores = make(map[string][]Conexion)   // conexiones CoAP
	mutexCoAP      sync.Mutex                    // Mutex para proteger el acceso a `clientesPorTopico`
)

// Iniciar el servidor CoAP
func IniciarCoAP (puerto string) {
	r := mux.NewRouter()
	// Manejador para cualquier ruta
	r.DefaultHandle(mux.HandlerFunc(manejadorCoAP))

	// Iniciar el servidor en el puerto especificado
	loggerPrint(LOG_COAP, "Iniciando servidor CoAP en :" + puerto)
	err := coap.ListenAndServe("udp", ":" + puerto, r)
	if err != nil {
		loggerFatal(LOG_COAP, "Error al iniciar el servidor: %v", err)
	}
}

// handleAll maneja todas las solicitudes CoAP, independientemente de la ruta
func manejadorCoAP(w mux.ResponseWriter, r *mux.Message) {
	ruta, err := r.Path()
	if err != nil {
		loggerPrint(LOG_COAP, "Error al obtener la ruta: %v %v", r.Code(), err)
		return
	}

	metodo := r.Code()
	loggerPrint(LOG_COAP, "Solicitud recibida: Método %v, Ruta %v", metodo, ruta)

	// obtengo si tiene observe
	obs, err := r.Options().Observe()

	// Responder según el método
	switch {
		// suscribirse
		case metodo == codes.GET && err == nil && obs == 0:
			manejarSuscripcionCoAP(w, r, ruta)
		// desuscribirse
		case metodo == codes.GET && err == nil && obs != 0:
			eliminarSuscripcionCoAP(w, r, ruta)
		// publicar
		case metodo == codes.POST:
			// Obtener la carga útil de la solicitud, si hay alguna
			var mensaje Mensaje
			cuerpo, err := r.Message.ReadBody()
			if err != nil {
				loggerPrint(LOG_COAP, "Error al procesar el cuerpo de la solicitud: "+ err.Error())
				return
			}
			err = json.Unmarshal(cuerpo, &mensaje)
			if err != nil {
				loggerPrint(LOG_COAP, "Error al convertir el cuerpo de la solicitud: "+ err.Error())
				return
			}
			manejarPublicacionCoAP(w, r, ruta, mensaje)
		default:
			loggerPrint(LOG_COAP, "Método no soportado: %v", metodo)
			err := w.SetResponse(codes.MethodNotAllowed, message.TextPlain,  bytes.NewReader([]byte("Método no soportado")))
			if err != nil {
				loggerPrint(LOG_COAP, "Error al enviar respuesta: %v", err)
			}
	}
}

// manejarSuscripcionCoAP maneja las solicitudes GET con observe
func manejarSuscripcionCoAP (w mux.ResponseWriter, r *mux.Message, topico string) {

	// agrego observadores
	loggerPrint(LOG_COAP, "Agregando observador")
	mutexCoAP.Lock()
	datosConexion := Conexion{w.Conn(), r.Context(), r.Token()}
	observadores[topico] = append(observadores[topico], datosConexion)
	mutexCoAP.Unlock()

	// enviar respuesta
	err :=  enviarRespuesta(w.Conn(), r.Token(), Mensaje{}, valor.Add(1))
	if err != nil {
		loggerPrint(LOG_COAP, "Error en transmitir: %v", err)
	}
}

// manejarPublicacionCoAP envía una publicación a los observadores de una ruta
func manejarPublicacionCoAP (w mux.ResponseWriter, r *mux.Message, ruta string, payload Mensaje) {

	err := w.SetResponse(codes.Created, message.TextPlain,  nil)
	if err != nil {
		loggerPrint(LOG_COAP, "Error al enviar respuesta: %v", err)
	}		

	// notifico a todos los observadores
	mutexCoAP.Lock()
	for _, o := range observadores[ruta] {
		enviarRespuesta(o.conexion, o.token, payload, valor.Add(1))
	}
	mutexCoAP.Unlock()
	// enviar publicaciones a los otros protocolos
	payload.Original = false
	enviarHTTP(LOG_COAP, payload)
	enviarMQTT(LOG_COAP, payload)
}

func eliminarSuscripcionCoAP (w mux.ResponseWriter, r *mux.Message, ruta string) {
	err := enviarRespuesta(w.Conn(), r.Token(), Mensaje{}, -1)
	if err != nil {
		loggerPrint(LOG_COAP, "Error al enviar respuesta: %v", err)
	}
}

func enviarRespuesta(cc mux.Conn, token []byte, mensaje Mensaje, obs int64) error {
	m := cc.AcquireMessage(cc.Context())
	defer cc.ReleaseMessage(m)
	m.SetCode(codes.Content)
	m.SetToken(token)
	mensajeBytes, err := json.Marshal(mensaje)
	if err != nil {
		loggerPrint(LOG_COAP, "Error al convertir el mensaje a []byte: %v", err)
		return err
	}
	m.SetBody(bytes.NewReader(mensajeBytes))
	m.SetContentFormat(message.TextPlain)
	if obs >= 0 {
		m.SetObserve(uint32(obs))
	}
	return cc.WriteMessage(m)
}
