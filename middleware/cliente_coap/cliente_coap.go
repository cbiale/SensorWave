package cliente_coap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/pool"
	"github.com/plgd-dev/go-coap/v3/udp"
	"github.com/plgd-dev/go-coap/v3/udp/client"
	obs "github.com/plgd-dev/go-coap/v3/net/client"
)

// tipo del cliente
type ClienteCoAP struct {
	cliente *client.Conn
}

// tipo de función de callback generica de bibliotecas del middleware
type CallbackFunc func(topico string, payload interface{})

// almacena suscripciones del cliente
var observaciones = make(map[string]obs.Observation)

// almacena si el mensaje es original o replica
type Mensaje struct {
    Original bool `json:"original"`
    Topico   string `json:"topico"`
    Payload  []byte `json:"payload"`
}

// conectar cliente
func Conectar(direccion string, puerto string) *ClienteCoAP {
	servidor := direccion + ":" + puerto
	log.Println("Conectandose a: ", servidor)
	cliente, err := udp.Dial(servidor)
	if err != nil {
		log.Fatalf("Error al conectarse: %v", err)
	}
	return &ClienteCoAP{cliente: cliente};
}

// cerrar cliente
func (c *ClienteCoAP) Desconectar() {
	c.cliente.Close()
}

// publicar
func (c *ClienteCoAP) Publicar(topico string, payload interface{}) {
	var data []byte
    switch v := payload.(type) {
    case string:
        data = []byte(v) // Si es un string, convertir directamente a []byte
    case []byte:
        data = v // Si ya es []byte, usarlo directamente
    case int, int32, int64, float32, float64:
        data = []byte(fmt.Sprintf("%v", v)) // Convertir números a string y luego a []byte
    default:
        // Para otros tipos, usar JSON como formato de serialización
        var err error
        data, err = json.Marshal(v)
        if err != nil {
            log.Fatalf("Error al serializar el payload: %v", err)
        }
    }

	mensaje := Mensaje{Original: true, Topico: topico, Payload: data}

	// Serializar el mensaje a JSON
	mensajeBytes, err := json.Marshal(mensaje)
	if err != nil {
		log.Fatalf("Error al serializar el mensaje: %v", err)
	}

	// publicar en el recurso
	ctx := context.Background()
	_, err = c.cliente.Post(ctx, topico, message.TextPlain, bytes.NewReader(mensajeBytes))
	if err != nil {
		log.Fatalf("Error : %v", err)
	}
}

// suscribir a tópico
// A futuro si ya estoy suscripto, primero desuscribir y luego suscribir
func (c *ClienteCoAP) Suscribir(topico string, callback CallbackFunc) { 
	// subscribe al recurso
	ctx := context.Background()
	internalCallback := func(msg *pool.Message) {
        var mensaje Mensaje
		if p, err := msg.ReadBody(); err == nil && len(p) > 0 {
			err := json.Unmarshal(p, &mensaje)
			if err != nil {
				log.Fatalf("Error al procesar el cuerpo de la solicitud: "+ err.Error())
				return
			}
		}
		callback(topico, string(mensaje.Payload))
	}
	obs , err := c.cliente.Observe(ctx, topico, internalCallback)
	if err != nil {
		log.Fatalf("Error : %v", err)
	}
	observaciones[topico] = obs
}

// se desuscribe a un topico
func (c *ClienteCoAP) desuscribir(topico string) {
	obs, ok := observaciones[topico]
	if !ok {
		log.Printf("No hay observación activa en %s", topico)
		return
	}
	if err := obs.Cancel(context.Background()); err != nil {
		log.Printf("Error al cancelar %s: %v", topico, err)
		return
	}
	delete(observaciones, topico)
}
