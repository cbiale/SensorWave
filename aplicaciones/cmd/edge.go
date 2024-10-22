package main

import (
    "fmt"
    "time"
    MQTT "github.com/eclipse/paho.mqtt.golang"
)

// app de ejemplo
func main() {
    opciones := MQTT.NewClientOptions().AddBroker("tcp://broker.hivemq.com:1883")
    opciones.SetClientID("SensorWaveMQTT")

    cliente := MQTT.NewClient(opciones)
    if token := cliente.Connect(); token.Wait() && token.Error() != nil {
        panic(token.Error())
    }

    fmt.Println("Conectado al broker MQTT")

    if token := cliente.Subscribe("test/topic", 0, nil); token.Wait() && token.Error() != nil {
        fmt.Println(token.Error())
        return
    }

    token := cliente.Publish("test/topic", 0, false, "Hello MQTT")
    token.Wait()

    time.Sleep(3 * time.Second)

    cliente.Disconnect(250)
    fmt.Println("Desconectado del broker MQTT")
}