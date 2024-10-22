package main

import (
	"fmt"
	"os"
	"os/signal"
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// app de ejemplo
func main() {
	opciones := MQTT.NewClientOptions().AddBroker("tcp://localhost:1883")
	opciones.SetClientID("SensorWaveMQTT")

	cliente := MQTT.NewClient(opciones)
	if token := cliente.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	fmt.Println("Conectado al broker MQTT")

	// el sistema debe suscribirse al topic datos/# y cuando llegan datos
	// almacenarlos en una base de datos sqlite

	// Callback function to handle incoming messages
	manejador := func(cliente MQTT.Client, mensaje MQTT.Message) {
		// Store the received data in a SQLite database
		// ...
	}

	// define la funcion manejador al llegar mensajes
	cliente.SetDefaultPublishHandler(manejador)

	// como hacer para que no termine el programa
	// hasta que presione ctrl-c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	fmt.Println("Desconectadose del broker MQTT")
	cliente.Disconnect()

}
