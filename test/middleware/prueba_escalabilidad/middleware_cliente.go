package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cbiale/sensorwave/middleware"
	sw_coap "github.com/cbiale/sensorwave/middleware/cliente_coap"
	sw_http "github.com/cbiale/sensorwave/middleware/cliente_http"
	sw_mqtt "github.com/cbiale/sensorwave/middleware/cliente_mqtt"
)

type ClienteConProtocolo struct {
    Cliente  middleware.Cliente
    Protocolo string
}

var (
	cliente_p, cliente_s middleware.Cliente
	protocoloPublicador, protocoloSuscriptor string
	CANTIDAD = 180
	TIEMPO = 1
	valores []int = make([]int, 2)
)

func main() {

	// Si se reciben dos argumentos
	// el primero es la cantidad de publicadores y el segundo la cantidad de suscriptores
	// si no se reciben argumentos o una cantidad distinta, error fatal
	if len(os.Args) != 3 {
		log.Fatal("Se deben recibir dos argumentos: la cantidad de publicadores y la cantidad de suscriptores")
	}
	
	for i := 0; i < 2; i++ {
		valores[i], _ = strconv.Atoi(os.Args[i+1])
		if valores[i] <= 0 {
			log.Fatal("La cantidad de publicadores y suscriptores debe ser mayor a 0")
		}
	}
	
	var publicadores []ClienteConProtocolo
	var suscriptores []ClienteConProtocolo

    // Creo publicadores y suscriptores
    for j := 0; j < valores[0]; j++ {
        publicadores = append(publicadores, ClienteConProtocolo{sw_http.Conectar("localhost", "8080"), "HTTP"})
        publicadores = append(publicadores, ClienteConProtocolo{sw_coap.Conectar("localhost", "5683"), "CoAP"})
        publicadores = append(publicadores, ClienteConProtocolo{sw_mqtt.Conectar("localhost", "1883"), "MQTT"})
    }
    for j := 0; j < valores[1]; j++ {
        suscriptores = append(suscriptores, ClienteConProtocolo{sw_http.Conectar("localhost", "8080"), "HTTP"})
        suscriptores = append(suscriptores, ClienteConProtocolo{sw_coap.Conectar("localhost", "5683"), "CoAP"})
        suscriptores = append(suscriptores, ClienteConProtocolo{sw_mqtt.Conectar("localhost", "1883"), "MQTT"})
    }

    for _, sub := range suscriptores {
        // Suscribo a los suscriptores
        sub.Cliente.Suscribir("/test", func(topico string, mensaje interface{}) {
            manejadorTest(topico, mensaje, sub.Protocolo)
        })
    }

	// espero un tiempo para que el suscriptor se suscriba
	time.Sleep(time.Duration(5) * time.Second)

    // Publicaciones
    for i := 0; i < CANTIDAD; i++ {
        for _, pub := range publicadores {
            go pub.Cliente.Publicar("/test", fmt.Sprintf("%d,%s", time.Now().UnixNano(), pub.Protocolo))
        }
        time.Sleep(time.Duration(TIEMPO) * time.Second)
    }
    time.Sleep(time.Duration(TIEMPO*5) * time.Second)
}

func manejadorTest(topico string, mensaje interface{}, receptor string) {
    recibido := time.Now().UnixNano()
    cadena := mensaje.(string)
    partes := strings.Split(cadena, ",")
    enviado, _ := strconv.ParseInt(partes[0], 10, 64)
    emisor := string(partes[1])

    // Almacenar en un archivo CSV el tiempo de envío, de llegada, emisor y receptor
    almacenar(enviado, recibido, emisor, receptor)
    log.Printf("%d,%d,%d,%s,%s\n", enviado, recibido, recibido-enviado, emisor, receptor)
}

func almacenar(enviado, recibido int64, emisor, receptor string) {
    // Convertir los valores de la lista a strings y concatenarlos
    nombreArchivo := fmt.Sprintf("datos_escalabilidad_%d_%d_middleware.csv", valores[0], valores[1])
    archivo, err := os.OpenFile(nombreArchivo, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
        emisor, // Emisor
        receptor, // Receptor
    })
    if err != nil {
        log.Fatalf("Error al escribir en el archivo CSV: %v", err)
    }
}