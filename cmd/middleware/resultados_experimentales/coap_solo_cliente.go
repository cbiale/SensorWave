package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/udp"
)

var (
	CANTIDAD = 500
	TIEMPO   = 1
)

// Iniciar el servidor CoAP
func main () { 
	enviado := int64(0)

	// validar que reciba un argumento desde la linea de comandos
	// de no suceder error fatal
	if len(os.Args) != 2 {
		log.Fatal("Se debe recibir un argumento: el metodo de prueba (get o post)") 
	}
	// validar que el argumento sea get o post
	if os.Args[1] != "get" && os.Args[1] != "post" {
		log.Fatal("El argumento debe ser get o post")
	}

	cliente, err := udp.Dial("localhost:5683")
	if err != nil {
		log.Fatalf("Error al conectarse: %v", err)
	}
	defer cliente.Close()

	for i := 0; i < CANTIDAD; i++ {
        // Enviar la solicitud CoAP Get
		ctx := context.Background()
		if os.Args[1] == "post" {
			// Enviar un mensaje POST
			enviado = time.Now().UnixNano()
			_, err = cliente.Post(ctx, "/test", message.TextPlain, nil)
			if err != nil {
				log.Fatalf("Error al enviar el mensaje: %v", err)
			}
		} else {
			// Enviar un mensaje GET
			enviado = time.Now().UnixNano()
			_, err = cliente.Get(ctx, "/test")
			if err != nil {
				log.Fatalf("Error al enviar el mensaje: %v", err)
			}
		}

		recibido := time.Now().UnixNano()

		almacenar(enviado, recibido)
		log.Printf("%d,%d,%d\n", enviado, recibido, recibido-enviado)

		time.Sleep(time.Duration(TIEMPO) * time.Second)
	}
}

func almacenar(enviado int64, recibido int64) {
	archivo, err := os.OpenFile("datos_coap_" + os.Args[1] + "_solo.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error al abrir el archivo: %v", err)
	}
	defer archivo.Close()

	// Crear un escritor CSV
	writer := csv.NewWriter(archivo)
	defer writer.Flush()

	// Escribir los datos en el archivo CSV
	err = writer.Write([]string{
		fmt.Sprintf("%d", enviado), // Tiempo de envío
		fmt.Sprintf("%d", recibido), // Tiempo de llegada
		fmt.Sprintf("%d", recibido - enviado), // Latencia
	})
	if err != nil {
		log.Fatalf("Error al escribir en el archivo CSV: %v", err)
	}
}
