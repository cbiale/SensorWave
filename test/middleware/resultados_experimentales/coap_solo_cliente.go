package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/plgd-dev/go-coap/v3/message"
	"github.com/plgd-dev/go-coap/v3/message/pool"
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
		log.Fatal("Se debe recibir un argumento: (pub o sub)") 
	}
	// validar que el argumento sea pub o sub
	if os.Args[1] != "pub" && os.Args[1] != "sub" {
		log.Fatal("El argumento debe ser pub o sub")
	}

	// crear el cliente CoAP
	cliente, err := udp.Dial("localhost:5683")
	if err != nil {
		log.Fatalf("Error al conectarse: %v", err)
	}
	defer cliente.Close()
	ctx := context.Background()

    if os.Args[1] == "pub" {
		for i := 0; i < CANTIDAD; i++ {
            // Enviar un mensaje POST
            enviado = time.Now().UnixNano()
			_, err = cliente.Post(ctx, "/test", message.TextPlain, bytes.NewReader([]byte(fmt.Sprintf("%d", enviado))))
			if err != nil {
				log.Fatalf("Error al enviar el mensaje: %v", err)
			}
            log.Printf("Mensaje enviado con éxito: %d\n", i)
			time.Sleep(time.Duration(TIEMPO) * time.Second)
		}
	}  else {

		internalCallback := func(msg *pool.Message) {
			p, err := msg.ReadBody()
			if err != nil {
				log.Printf("Error al leer el cuerpo del mensaje: %v", err)
				return
			}

			// si es un mensaje interno, no lo procesamos
			if (p == nil) {
				log.Printf("Mensaje interno, ignorando")
				return
			}
			
			// si no es un mensaje interno, lo procesamos
			recibido := time.Now().UnixNano()
			// convertir el mensaje a int64
			enviado, err := strconv.ParseInt(string(p), 10, 64)
			if err != nil {
				log.Fatalf("Error al convertir el mensaje a int64: %v", err)
				return
			}
			almacenar(enviado, recibido)
			log.Printf("%d,%d,%d\n", enviado, recibido, recibido-enviado)
		}

		_ , err := cliente.Observe(ctx, "/test", internalCallback)
		if err != nil {
			log.Fatalf("Error : %v", err)
		}
		select {}
	}
}

func almacenar(enviado int64, recibido int64) {
	archivo, err := os.OpenFile("datos_coap_solo.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
