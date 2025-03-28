package main

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/cockroachdb/pebble"
)

// Función para aplicar compresión Delta
func compresionDelta(valores []int64) []byte {
	// Crear un slice de bytes para almacenar los datos comprimidos
	compressed := make([]byte, 8*len(valores))
	// Guardar el primer valor sin comprimir
	base := valores[0]
	// Guardar el primer valor en el slice de bytes
	binary.LittleEndian.PutUint64(compressed[0:8], uint64(base))
	for i := 1; i < len(valores); i++ {
		delta := valores[i] - valores[i-1]
		binary.LittleEndian.PutUint64(compressed[i*8:(i+1)*8], uint64(delta))
	}
	return compressed
}

func main() {
	db, err := pebble.Open("sensor.db", &pebble.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Simulamos datos de un sensor
	values := []int64{100, 99, 105, 110, 120} // Datos simulados
	datosComprimidos := compresionDelta(values)

	// Guardamos los datos comprimidos
	err = db.Set([]byte("sensor:123"), datosComprimidos, pebble.Sync)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Datos comprimidos guardados en Pebble")
	fmt.Println("Datos comprimidos:", datosComprimidos)
}
