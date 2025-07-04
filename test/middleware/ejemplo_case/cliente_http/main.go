package main

import (
	"fmt"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_http"
)

var cliente *sw_cliente.ClienteHTTP

func main() {
	// crea un nuevo cliente
	cliente = sw_cliente.Conectar("localhost", "8080")
	defer cliente.Desconectar()

	// suscribirse a /temperatura
	cliente.Suscribir("/temperatura", manejadorTemperatura)
	// suscribirse a /humedad
	cliente.Suscribir("/humedad", manejadorHumedad)
	select {}
}

// manejadorTemperatura es el manejador de mensajes para el tópico /temperatura
func manejadorTemperatura(topico string, mensaje interface{}) {
	fmt.Printf("Mensaje en /temperatura: %v\n", mensaje)
}

// manejadorHumedad es el manejador de mensajes para el tópico /humedad
func manejadorHumedad(topico string, mensaje interface{}) {
	fmt.Printf("Mensaje en /humedad: %v\n", mensaje)
}
