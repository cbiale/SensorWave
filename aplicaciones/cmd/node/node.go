// conectarse a un broker mqtt en localhost:1883
// que cada 5 segundos publique un mensaje en el topic "datos/1"
// con un valor aleatorio entre 0 y 40 de temperatura
// y otro valor aleatorio entre 0 y 100 de humedad

package main

import (
	"fmt"
	"math/rand"
	"time"
	
	"github.com/eclipse/paho.mqtt.golang"

	gen "sensorwave/gen"
)

func main() {

	opts := mqtt.NewClientOptions().AddBroker("tcp://localhost:1883")
	cliente := mqtt.NewClient(opts)
	if token := cliente.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	for {
		temperatura := rand.Intn(40)
		humedad := rand.Intn(100)
		
		msg := fmt.Sprintf("{\"temperatura\": %d, \"humedad\": %d}", temperatura, humedad)
		token := cliente.Publish("datos/1", 0, false, msg)
		token.Wait()
		time.Sleep(5 * time.Second)
	}
}

