package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var (
    CANTIDAD = 500
    TIEMPO = 1
)

func main() {

	enviado := int64(0)
    resp := &http.Response{}
    var err error

	// validar que reciba un argumento desde la linea de comandos
	// de no suceder error fatal
	if len(os.Args) != 2 {
		log.Fatal("Se debe recibir un argumento: el metodo de prueba (get o post)") 
	}
	// validar que el argumento sea get o post
	if os.Args[1] != "get" && os.Args[1] != "post" {
		log.Fatal("El argumento debe ser get o post")
	}

    for i := 0; i < CANTIDAD; i++ {

        if os.Args[1] == "post" {
            // Enviar un mensaje POST
            enviado = time.Now().UnixNano()
            resp, err = http.Post("http://localhost:8080/test", "application/json", nil)
            if err != nil {
                log.Printf("Error al enviar la solicitud: %v\n", err)
                continue
            }
        } else {
            // Enviar un mensaje GET
            enviado = time.Now().UnixNano()
            resp, err = http.Get("http://localhost:8080/test")
            if err != nil {
                log.Printf("Error al enviar la solicitud: %v\n", err)
                continue
            }
        }
        recibido := time.Now().UnixNano()
        resp.Body.Close()
        // Almacenar en un archivo CSV el tiempo de envío y de llegada del mensaje
        almacenar(enviado, recibido)
        log.Printf("%d,%d,%d\n", enviado, recibido, recibido-enviado)

        time.Sleep(time.Duration(TIEMPO) * time.Second)
    }
}

func almacenar(enviado int64, recibido int64) {
	archivo, err := os.OpenFile("datos_http_"+ os.Args[1] + "_solo.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
