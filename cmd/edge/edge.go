package main

import (
	"os"
	"os/signal"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"go.uber.org/zap"
	"github.com/cbiale/sensorwave/sensorwave"
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

	// inicializa sensorwave
	logger.Info("Inicializando SensorWave")

	db, err := sensorwave.Init("midb.db", "localhost:9000", "miniominio", "miniominio", "gzip", 1000, 40)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	logger.Info("SensorWave inicializado")

	db.RegisterSensor("Temperatura ambiente", "°C")
	db.RegisterSensor("Humedad ambiente", "%")
	db.RegisterSensor("Nivel de alimento", "%")
	db.RegisterSensor("Nivel de agua", "%")

	sensors, err := db.GetSensors()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.Info("Sensores registrados", zap.Any("sensores", sensors))


	db.RegisterActuator("Ventilador", []string{"Encender", "Apagar"})
	db.RegisterActuator("Bomba de agua", []string{"Encender", "Apagar"})

	actuators, err := db.GetActuators()

	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.Info("Actuadores registrados", zap.Any("actuadores", actuators))

	// función que maneja los mensajes entrantes
	manejador := func(cliente MQTT.Client, mensaje MQTT.Message) {
		logger.Info("Mensaje recibido",
			zap.String("topico", mensaje.Topic()),
			zap.ByteString("mensaje", mensaje.Payload()),
		)
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
	// cierra sensorwave
	db.Close()
}
