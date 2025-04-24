package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

func main() {

	// definir los tipos de combinaciones
	rangosPublicadores := []int{1, 10, 20}
	rangosSuscriptores := []int{1, 10, 20}

	// recorrer las combinaciones
	for _, publicador := range rangosPublicadores {
		for _, suscriptor := range rangosSuscriptores {
			cmdServidor := exec.Command("./middleware_servidor")
			fmt.Println("Prueba publicadores: ", publicador, ", suscriptores: ", suscriptor)
			cmdCliente := exec.Command("./middleware_cliente", strconv.Itoa(publicador), strconv.Itoa(suscriptor))

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
