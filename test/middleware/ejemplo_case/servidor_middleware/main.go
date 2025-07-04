package main

import (
	servidor_sw "github.com/cbiale/sensorwave/middleware/servidor"
)

func main() {
	// Iniciar el servidor HTTP, MQTT y CoAP
	go servidor_sw.IniciarHTTP("8080")
	go servidor_sw.IniciarMQTT("1883")
	go servidor_sw.IniciarCoAP("5683")
	select {}
}
