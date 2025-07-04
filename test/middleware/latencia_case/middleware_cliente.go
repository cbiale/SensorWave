package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cbiale/sensorwave/middleware"
	sw_coap "github.com/cbiale/sensorwave/middleware/cliente_coap"
	sw_http "github.com/cbiale/sensorwave/middleware/cliente_http"
	sw_mqtt "github.com/cbiale/sensorwave/middleware/cliente_mqtt"
)

var (
	cliente_p, cliente_s                     middleware.Cliente
	protocoloPublicador, protocoloSuscriptor string
	CANTIDAD                                 = 500
	TIEMPO                                   = 1
)

func main() {

	// Si se reciben dos argumentos
	// el primero es el protocolo publicador y el segundo el suscriptor
	// si no se reciben argumentos o una cantidad distinta, error fatal
	if len(os.Args) != 3 {
		log.Fatal("Se deben recibir dos argumentos: el protocolo publicador y el suscriptor")
	}

	protocoloPublicador = os.Args[1]
	protocoloSuscriptor = os.Args[2]

	if protocoloPublicador != "http" && protocoloPublicador != "mqtt" && protocoloPublicador != "coap" {
		log.Fatal("El protocolo publicador debe ser http, mqtt o coap")
	}
	if protocoloSuscriptor != "http" && protocoloSuscriptor != "mqtt" && protocoloSuscriptor != "coap" {
		log.Fatal("El protocolo suscriptor debe ser http, mqtt o coap")
	}

	// si el protocolo publicador es http
	if protocoloPublicador == "http" {
		// crea un nuevo cliente http
		cliente_p = sw_http.Conectar("localhost", "8080")
		defer cliente_p.Desconectar()
	} else if protocoloPublicador == "mqtt" {
		// crea un nuevo cliente mqtt
		cliente_p = sw_mqtt.Conectar("localhost", "1883")
		defer cliente_p.Desconectar()
	} else if protocoloPublicador == "coap" {
		// crea un nuevo cliente coap
		cliente_p = sw_coap.Conectar("localhost", "5683")
		defer cliente_p.Desconectar()
	}

	// si el protocolo suscriptor es http
	if protocoloSuscriptor == "http" {
		// crea un nuevo cliente http
		cliente_s = sw_http.Conectar("localhost", "8080")
		defer cliente_s.Desconectar()
	} else if protocoloSuscriptor == "mqtt" {
		// crea un nuevo cliente mqtt
		cliente_s = sw_mqtt.Conectar("localhost", "1883")
		defer cliente_s.Desconectar()
	} else if protocoloSuscriptor == "coap" {
		// crea un nuevo cliente coap
		cliente_s = sw_coap.Conectar("localhost", "5683")
		defer cliente_s.Desconectar()
	}

	// suscribo el cliente al topico /test
	cliente_s.Suscribir("/test", manejadorTest)

	// espero un tiempo para que el suscriptor se suscriba
	time.Sleep(time.Duration(TIEMPO) * time.Second)

	// publicaciones
	for i := 0; i < CANTIDAD; i++ {
		// publicar un mensaje en /test
		cliente_p.Publicar("/test", time.Now().UnixNano())
		// espero un tiempo antes de enviar el siguiente mensaje
		time.Sleep(time.Duration(TIEMPO) * time.Second)
	}

	// espero un tiempo para que el suscriptor reciba los mensajes
	time.Sleep(time.Duration(TIEMPO*3) * time.Second)
}

func manejadorTest(topico string, mensaje interface{}) {
	recibido := time.Now().UnixNano()
	cadena := mensaje.(string)
	enviado, _ := strconv.ParseInt(cadena, 10, 64)

	// almacenar en un archivo csv el tiempo de envio y de llegada del mensaje
	almacenar(enviado, recibido)
	log.Printf("%d,%d,%d\n", enviado, recibido, recibido-enviado)
}

func almacenar(enviado, recibido int64) {
	archivo, err := os.OpenFile("datos_"+protocoloPublicador+"_"+protocoloSuscriptor+"_middleware.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error al abrir el archivo: %v", err)
	}
	defer archivo.Close()

	// Crear un escritor CSV
	writer := csv.NewWriter(archivo)
	defer writer.Flush()

	// Escribir los datos en el archivo CSV
	err = writer.Write([]string{
		fmt.Sprintf("%d", enviado),          // Tiempo de envío
		fmt.Sprintf("%d", recibido),         // Tiempo de llegada
		fmt.Sprintf("%d", recibido-enviado), // Latencia
	})
	if err != nil {
		log.Fatalf("Error al escribir en el archivo CSV: %v", err)
	}
}
