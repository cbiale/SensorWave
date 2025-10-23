package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_nats"
)

var cliente *sw_cliente.ClienteNATS

func main() {
	// crea un nuevo cliente
	var err error
	cliente, err = sw_cliente.Conectar("localhost", "4222")
	if err != nil {
		log.Fatal("Error al conectar:", err)
	}
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
