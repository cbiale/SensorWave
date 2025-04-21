package main

import (
	"fmt"
	"os/exec"
	"time"
)

func main() {

	// definir los tres tipos de protocolos en un arreglo
	protocolos := []string{"coap", "http", "mqtt"}

	// recorrer los protocolos
	for _, protocoloPublicador := range protocolos {
		for _, protocoloSuscriptor := range protocolos {
			cmdServidor := exec.Command("./middleware_servidor")
			cmdCliente := exec.Command("./middleware_cliente", protocoloPublicador, protocoloSuscriptor)

			// ejecutar el servidor
			err := cmdServidor.Start()
			if err != nil {
				fmt.Println("Error al iniciar el servidor:", err)
				return
			}
			fmt.Println("Servidor iniciado")

			time.Sleep(5 * time.Second) // esperar 5 segundos para que el servidor esté listo
			
			// ejecutar el cliente y esperar a que termine
			err = cmdCliente.Run()
			if err != nil {
				fmt.Println("Error al ejecutar el cliente:", err)
				return
			}
			fmt.Println("Cliente ejecutado")

			// detener el servidor
			err = cmdServidor.Process.Kill()
			if err != nil {
				fmt.Println("Error al detener el servidor:", err)
				return
			}
			fmt.Println("Servidor detenido")
		}
	}
}