package main

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"

	gen "sensorwave/gen"
)

// app de ejemplo
func main() {
	logger := zap.Must(zap.NewProduction())
	defer logger.Sync()

	// opciones a pasar al cliente MQTT
	opciones := MQTT.NewClientOptions().AddBroker("tcp://localhost:1883")
	opciones.SetClientID("SensorWaveMQTT")

	// crea un cliente MQTT
	cliente := MQTT.NewClient(opciones)
	if token := cliente.Connect(); token.Wait() && token.Error() != nil {
		logger.Error(token.Error().Error())
		os.Exit(1)
	}

	logger.Info("Conectado al broker MQTT")

	// función que maneja los mensajes entrantes
	manejador := func(cliente MQTT.Client, mensaje MQTT.Message) {
		logger.Info("Mensaje recibido",
			zap.String("topico", mensaje.Topic()),
			zap.ByteString("mensaje", mensaje.Payload()),
		)

		// el payload viene en formato FlatBuffers
		// se puede deserializar con el esquema generado
		// por flatc ubicado en el archivo ./gen/payload_generated.go
		// deserializa el payload
		buf := mensaje.Payload()
		payload := gen.GetRootAsPayload(buf, 0)
		id := payload.Id(0)
		timestamp := payload.Timestamp()
		valores := payload.ValoresLength()
		for i := 0; i < valores; i++ {
			claveValor := new(gen.ClaveValor)
			payload.Valores(claveValor, i)
			clave := claveValor.Clave()

			fmt.Println("ID:", id)
			fmt.Println("Timestamp:", time.Unix(0, timestamp))
			fmt.Println("Clave:", clave)

			var valorBool gen.ValorBool
			tablaBool := valorBool.Table()
			if claveValor.Valor(&tablaBool) {
				// si el valor es un booleano
				fmt.Println("Valor: ", valorBool.Valor())
			}
			var valorFloat gen.ValorFloat
			tablaFloat := valorFloat.Table()
			if claveValor.Valor(&tablaFloat) {
				fmt.Println("Valor: ", valorFloat.Valor())
			}

		}
	}

	// se suscribe a todos los mensajes con tópicos que comiencen con "datos/"
	// el segundo parámetro es el nivel de QoS
	// 0: al menos una vez
	// el tercer parámetro es la función manejadora de mensajes
	cliente.Subscribe("datos/#", 0, manejador)

	// termina el programa al presionar ctrl-C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	// se desconecta del broker MQTT
	logger.Info("Desconectadose del broker MQTT")
	cliente.Disconnect(1000)

}
