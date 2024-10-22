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

	// función que maneja los mensajes entrantes
	manejador := func(cliente MQTT.Client, mensaje MQTT.Message) {
		// Store the received data in a SQLite database
		// ...
	}

	// define la función que maneja los mensajes entrantes
	cliente.AddRoute("datos/#", manejador)

	// termina el programa al presionar ctrl-C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	fmt.Println("Desconectadose del broker MQTT")
	cliente.Disconnect(1000)

}
