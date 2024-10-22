package main

import (
	"fmt"
	"os"
	"os/signal"
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// app de ejemplo
func main() {
	// opciones a pasar al cliente MQTT
	opciones := MQTT.NewClientOptions().AddBroker("tcp://localhost:1883")
	opciones.SetClientID("SensorWaveMQTT")

	// crea un cliente MQTT
	cliente := MQTT.NewClient(opciones)
	if token := cliente.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	fmt.Println("Conectado al broker MQTT")

	// función que maneja los mensajes entrantes
	manejador := func(cliente MQTT.Client, mensaje MQTT.Message) {
		fmt.Printf("Mensaje recibido: %s de %s\n", mensaje.Payload(), mensaje.Topic())
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
	fmt.Println("Desconectadose del broker MQTT")
	cliente.Disconnect(1000)

}
