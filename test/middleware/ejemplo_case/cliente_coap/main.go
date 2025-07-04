package main

import (
	"fmt"
	"math/rand"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_coap"
)

var cliente *sw_cliente.ClienteCoAP

func main() {
	// crea un nuevo cliente
	cliente = sw_cliente.Conectar("localhost", "5683")
	defer cliente.Desconectar()

	for i := 0; i < 5; i++ {
		// publicar un mensaje en /temperatura
		valor := aleatorio(10, 100)
		fmt.Printf("Publicando en /humedad: %d\n", valor)
		cliente.Publicar("/humedad", valor)
		time.Sleep(5 * time.Second)
	}
}

// aleatorio genera un número aleatorio entre min y max
func aleatorio(min, max int) int {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	return rand.Intn(max-min) + min
}
