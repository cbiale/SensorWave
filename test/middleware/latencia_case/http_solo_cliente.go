package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
		log.Fatal("Se debe recibir un argumento: (pub o sub)") 
	}
	// validar que el argumento sea pub o sub
	if os.Args[1] != "pub" && os.Args[1] != "sub" {
		log.Fatal("El argumento debe ser pub o sub")
	}

    if os.Args[1] == "pub" {
        for i := 0; i < CANTIDAD; i++ {
            // Enviar un mensaje POST
            enviado = time.Now().UnixNano()
            // adjuntar el tiempo de envio al mensaje como un string
            resp, err = http.Post("http://localhost:8080/test", "application/json", bytes.NewReader([]byte(strconv.FormatInt(enviado, 10))))
            if err != nil {
                log.Printf("Error al enviar la solicitud: %v\n", err)
                continue
            }
            resp.Body.Close()
            log.Printf("Mensaje enviado con éxito: %d\n", i)
            time.Sleep(time.Duration(TIEMPO) * time.Second)    
        }
    } else {
        go func() {
            resp, err := http.Get("http://localhost:8080/test")
            if err != nil {
                log.Fatalf("Error al realizar el GET: %v", err)
            }
            defer resp.Body.Close()

            // Crear un lector para el flujo SSE
            reader := bufio.NewReader(resp.Body)

            // Leer el flujo SSE 
            for {
                linea, err := reader.ReadString('\n')
                if err != nil {
                    if err == io.EOF {
                        log.Println("Conexión SSE cerrada por el servidor")
                        break
                    }
                    log.Fatalf("Error al leer el flujo SSE: %v", err)
                }

                // Procesar solo líneas que comiencen con "data: "
                if strings.HasPrefix(linea, "data: ") {
                    datos := strings.TrimPrefix(linea, "data: ")
                    datos = strings.TrimSpace(datos) // Eliminar espacios en blanco
                    // en datos viene realmente un valor int64
                    recibido := time.Now().UnixNano()
                    enviado, err := strconv.ParseInt(datos, 10, 64)
                    if err != nil {
                        log.Fatalf("Error al convertir el dato a int64: %v", err)
                    }
                    almacenar(enviado, recibido)
                    log.Printf("%d,%d,%d\n", enviado, recibido, recibido-enviado)
                }
            }        
        }()
        select {}
    }
}

// almacenar guarda los datos en un archivo CSV
func almacenar(enviado int64, recibido int64) {
	archivo, err := os.OpenFile("datos_http_solo.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
