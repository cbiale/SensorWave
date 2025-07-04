package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var CANTIDAD = 500

func main() {

	servidor := "tcp://localhost:1883"
	opts := mqtt.NewClientOptions()
	opts.AddBroker(servidor)
	opts.SetClientID("sensorwave_cliente_publicador")

	// Crear el cliente MQTT publicador
	clientePublicador := mqtt.NewClient(opts)
	if token := clientePublicador.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Error al conectar al broker MQTT: %v", token.Error())
	}
	defer clientePublicador.Disconnect(250)

	opts.SetClientID("sensorwave_cliente_suscriptor")
	// Crear el cliente MQTT suscriptor
	clienteSuscriptor := mqtt.NewClient(opts)
	if token := clienteSuscriptor.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Error al conectar al broker MQTT: %v", token.Error())
	}
	defer clienteSuscriptor.Disconnect(250)

	if token := clienteSuscriptor.Subscribe("/test", 0, manejadorTest); token.Wait() && token.Error() != nil {
		log.Fatalf("Error: %v", token.Error())
	}
	time.Sleep(1 * time.Second)

	fmt.Println("--")
	for i := 0; i < CANTIDAD; i++ {
		// publicar un mensaje en /test
		if token := clientePublicador.Publish("/test", 0, false, fmt.Sprintf("%d", time.Now().UnixNano())); token.Wait() && token.Error() != nil {
			log.Fatalf("Error: %v", token.Error())
		}
		time.Sleep(1 * time.Second)
	}
	time.Sleep(3 * time.Second)
}

func manejadorTest(cliente mqtt.Client, mensaje mqtt.Message) {
	recibido := time.Now().UnixNano()
	cadena := string(mensaje.Payload())
	enviado, _ := strconv.ParseInt(cadena, 10, 64)

	// almacenar en un archivo csv el tiempo de envio y de llegada del mensaje
	almacenar(enviado, recibido)
	log.Printf("%d,%d,%d\n", enviado, recibido, recibido-enviado)
}

func almacenar(enviado int64, recibido int64) {
	archivo, err := os.OpenFile("datos_mqtt_solo.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
