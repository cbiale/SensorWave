package cliente_nats

import (
	"encoding/json"
	"log"

	"github.com/cbiale/sensorwave/middleware"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type ClienteNATS struct {
	conn          *nats.Conn
	suscripciones map[string]*nats.Subscription
	clienteID     string
}

func Conectar(direccion string, puerto string) (*ClienteNATS, error) {
	servidor := "nats://" + direccion + ":" + puerto
	nc, err := nats.Connect(servidor)
	if err != nil {
		log.Printf("Advertencia: No se pudo conectar a NATS en %s: %v", servidor, err)
		return nil, err
	}

	log.Printf("Conectado exitosamente a NATS en %s", servidor)
	return &ClienteNATS{
		conn:          nc,
		suscripciones: make(map[string]*nats.Subscription),
		clienteID:     uuid.New().String(),
	}, nil
}

func (c *ClienteNATS) Desconectar() {
	if c.conn != nil {
		c.conn.Close()
		log.Printf("Cliente NATS %s desconectado", c.clienteID)
	}
}

func (c *ClienteNATS) Publicar(topico string, mensaje interface{}) {
	if c == nil || c.conn == nil {
		return
	}

	mensajeEnvio := middleware.Mensaje{
		Original: true,
		Topico:   topico,
		Payload:  nil,
		Interno:  false,
	}

	switch v := mensaje.(type) {
	case []byte:
		mensajeEnvio.Payload = v
	case string:
		mensajeEnvio.Payload = []byte(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			log.Printf("Error serializando mensaje: %v", err)
			return
		}
		mensajeEnvio.Payload = data
	}

	data, err := json.Marshal(mensajeEnvio)
	if err != nil {
		log.Printf("Error serializando mensaje NATS: %v", err)
		return
	}

	err = c.conn.Publish("middleware."+topico, data)
	if err != nil {
		log.Printf("Error publicando en NATS: %v", err)
	}
}

func (c *ClienteNATS) Suscribir(topico string, manejador middleware.CallbackFunc) {
	if c == nil || c.conn == nil {
		return
	}

	subject := "middleware." + topico

	sub, err := c.conn.Subscribe(subject, func(m *nats.Msg) {
		var mensaje middleware.Mensaje
		err := json.Unmarshal(m.Data, &mensaje)
		if err != nil {
			log.Printf("Error deserializando mensaje NATS: %v", err)
			return
		}

		if !mensaje.Interno {
			manejador(mensaje.Topico, mensaje.Payload)
		}
	})

	if err != nil {
		log.Printf("Error suscribiéndose a %s: %v", subject, err)
		return
	}

	c.suscripciones[topico] = sub
	log.Printf("Cliente NATS suscrito a %s", subject)
}

func (c *ClienteNATS) Desuscribir(topico string) {
	if sub, existe := c.suscripciones[topico]; existe {
		err := sub.Unsubscribe()
		if err != nil {
			log.Printf("Error desuscribiéndose de %s: %v", topico, err)
		} else {
			delete(c.suscripciones, topico)
			log.Printf("Cliente NATS desuscrito de middleware.%s", topico)
		}
	}
}
