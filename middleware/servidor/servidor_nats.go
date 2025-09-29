package servidor

import (
	"encoding/json"

	"github.com/nats-io/nats.go"
)

const LOG_NATS = "NATS"

var clienteNATS *nats.Conn

// IniciarNATS inicia el cliente NATS
func IniciarNATS(puerto string) {
	loggerPrint(LOG_NATS, "Iniciando cliente NATS en :"+puerto)

	// Conectar al servidor NATS
	nc, err := nats.Connect("nats://localhost:" + puerto)
	if err != nil {
		loggerFatal(LOG_NATS, "Error conectando a NATS: %v", err)
	}
	clienteNATS = nc

	// Suscribirse como middleware/broker
	_, err = clienteNATS.Subscribe("middleware.>", manejarMensajeMiddleware)
	if err != nil {
		loggerFatal(LOG_NATS, "Error suscribiéndose a middleware.>: %v", err)
	}

	loggerPrint(LOG_NATS, "Cliente NATS conectado y suscrito a middleware.>")
}

// manejarMensajeMiddleware maneja mensajes recibidos en el middleware
func manejarMensajeMiddleware(m *nats.Msg) {
	var mensaje Mensaje
	err := json.Unmarshal(m.Data, &mensaje)
	if err != nil {
		loggerPrint(LOG_NATS, "Error al procesar mensaje: %v", err)
		return
	}

	loggerPrint(LOG_NATS, "Mensaje recibido en middleware para tópico: "+mensaje.Topico)

	// Distribuir a otros protocolos si es original
	if mensaje.Original {
		mensaje.Original = false
		go enviarCoAP(LOG_NATS, mensaje)
		go enviarHTTP(LOG_NATS, mensaje)
		go enviarMQTT(LOG_NATS, mensaje)
	}
}

// enviarNATS publica un mensaje en NATS
func enviarNATS(LOG string, payload Mensaje) {
	loggerPrint(LOG, ">> Publicando mensaje en NATS "+payload.Topico)

	// Serializar el mensaje a JSON
	mensajeBytes, err := json.Marshal(payload)
	if err != nil {
		loggerPrint(LOG, "Error al serializar mensaje: %v", err)
		return
	}

	// Publicar en NATS
	err = clienteNATS.Publish("middleware."+payload.Topico, mensajeBytes)
	if err != nil {
		loggerPrint(LOG, "Error publicando en NATS: %v", err)
	}
}
