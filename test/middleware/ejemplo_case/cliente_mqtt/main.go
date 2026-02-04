package main

import (
	"fmt"
	"math/rand"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_mqtt"
)

var cliente *sw_cliente.ClienteMQTT

func main() {
	// crea un nuevo cliente
	cliente = sw_cliente.Conectar("localhost", "1883")
	defer cliente.Desconectar()

	for i := 0; i < 5; i++ {
		// publicar un mensaje en /temperatura
		valor := aleatorio(10, 40)
		fmt.Printf("Publicando en /temperatura: %d\n", valor)
		cliente.Publicar("/temperatura", valor)
		time.Sleep(5 * time.Second)
	}
}

// aleatorio genera un número aleatorio entre min y max
func aleatorio(min, max int) int {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	return rand.Intn(max-min) + min
}
