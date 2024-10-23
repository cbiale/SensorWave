package main

import (
	"os"
	"os/signal"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
)

// app de ejemplo
func main() {
	logger := zap.Must(zap.NewProduction())
	defer logger.Sync()
	
	// opciones a pasar al cliente MQTT
	opciones := MQTT.NewClientOptions().AddBroker("tcp://localhost:1883")
	opciones.SetClientID("SensorWaveMQTT")

	// crea un cliente MQTT
	cliente := MQTT.NewClient(opciones)
	if token := cliente.Connect(); token.Wait() && token.Error() != nil {
		logger.Error(token.Error().Error())
		os.Exit(1)
	}

	logger.Info("Conectado al broker MQTT")

	// función que maneja los mensajes entrantes
	manejador := func(cliente MQTT.Client, mensaje MQTT.Message) {
		logger.Info("Mensaje recibido",
			zap.String("topico", mensaje.Topic()),
			zap.ByteString("mensaje", mensaje.Payload()),
		)
		
	}

	// se suscribe a todos los mensajes con tópicos que comiencen con "datos/"
	cliente.Subscribe("datos/#", 0, nil)
	// define la función que maneja los mensajes entrantes
	cliente.AddRoute("datos/#", manejador)

	// termina el programa al presionar ctrl-C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	// se desconecta del broker MQTT
	logger.Info("Desconectadose del broker MQTT")
	cliente.Disconnect(1000)

}
