package main

import (
	"os"
	"os/signal"
	"database/sql"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
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
		// por flatc ubicado en el archivo payload_generated.go
		
		
		// deserializa el payload
		// maneja la base de datos Sqlite
		// almacena los datos
		
		// crea la base de datos
		datos, err := sql.Open("sqlite3", "./sensores.db")
		if err != nil {
			logger.Fatal(err.Error())
		}
		defer datos.Close() 
		// inserta los datos (usar canales, tener un hilo que inserte los datos)

	}

	// se suscribe a todos los mensajes con tópicos que comiencen con "datos/"
	cliente.Subscribe("datos/#", 0, nil)
	// define la función que maneja los mensajes entrantes
	cliente.AddRoute("datos/#", manejador)

	// termina el programa al presionar ctrl-C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	// se desconecta del broker MQTT
	logger.Info("Desconectadose del broker MQTT")
	cliente.Disconnect(1000)

}
