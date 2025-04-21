package servidor

import (
	"encoding/json"
	"strings"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

const LOG_MQTT = "MQTT"

var clienteMQTT MQTT.Client

// Iniciar el servidor MQTT
func IniciarMQTT (puerto string) {	
	loggerPrint(LOG_MQTT, "Iniciando cliente MQTT en :" + puerto)

	// Crear un nuevo cliente MQTT
	opciones := MQTT.NewClientOptions().AddBroker("tcp://localhost:" + puerto)
	opciones.SetClientID("SensorWaveMQTT")

	// Conectar al servidor MQTT
	// crea un cliente MQTT
	clienteMQTT = MQTT.NewClient(opciones)
	if token := clienteMQTT.Connect(); token.Wait() && token.Error() != nil {
		loggerFatal(LOG_MQTT, "Error al conectar al servidor: %v", token.Error())
	}
	// Suscribirse al topico "#" (VER SI SE SUSCRIBE A OTRO TOPICO)
	clienteMQTT.Subscribe("#", 0, manejadorMQTT)

	loggerPrint(LOG_MQTT, "Cliente MQTT conectado y suscrito al tópico #")
}

// manejadorMQTT maneja los mensajes MQTT recibidos
func manejadorMQTT(cliente MQTT.Client, mensajeMQTT MQTT.Message) {
	if strings.HasPrefix(mensajeMQTT.Topic(), "$SYS/") {
		return // Ignora mensajes del sistema
	}
	loggerPrint(LOG_MQTT, "Mensaje recibido en el tópico " + mensajeMQTT.Topic())

    var mensaje Mensaje
    err := json.Unmarshal(mensajeMQTT.Payload(), &mensaje)
    if err != nil {
        loggerPrint(LOG_MQTT, "Error al procesar el cuerpo de la solicitud: "+ err.Error())
        return
    }

	// enviar publicaciones a los otros protocolos (si el mensaje es original)
	if mensaje.Original {
		mensaje.Original = false
		go enviarCoAP(LOG_MQTT, mensaje)
		go enviarHTTP(LOG_MQTT, mensaje)
	}
}

