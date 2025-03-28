package main

import (
	"fmt"
	"log"
	"os"

	"github.com/cockroachdb/pebble"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
)

// Definimos la estructura del archivo Parquet
type SensorData struct {
	Key   string `parquet:"name=key, type=BYTE_ARRAY"`
	Value string `parquet:"name=value, type=BYTE_ARRAY"`
}

func main() {
	// Abrir Pebble DB
	db, err := pebble.Open("sensor.db", &pebble.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Crear archivo Parquet
	fw, err := os.Create("export.parquet")
	if err != nil {
		log.Fatal("No se pudo crear el archivo Parquet:", err)
	}
	defer fw.Close()

	// Crear el escritor de Parquet
	parquetFile, err := parquet.NewFileWriter(fw, new(SensorData), 4)
	if err != nil {
		log.Fatal("Error al crear el escritor Parquet:", err)
	}
	pw, err := writer.NewParquetWriter(parquetFile, new(SensorData), 4)
	if err != nil {
		log.Fatal("Error al crear el escritor Parquet:", err)
	}
	pw.RowGroupSize = 128 * 1024 * 1024 // 128MB
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	// Leer datos de Pebble y escribir en Parquet
	iter, _ := db.NewIter(nil)
	for iter.First(); iter.Valid(); iter.Next() {
		record := SensorData{
			Key:   string(iter.Key()),
			Value: string(iter.Value()),
		}
		if err := pw.Write(record); err != nil {
			log.Fatal("Error al escribir en Parquet:", err)
		}
	}
	iter.Close()

	// Cerrar el escritor
	if err := pw.WriteStop(); err != nil {
		log.Fatal("Error al cerrar el archivo Parquet:", err)
	}
	fmt.Println("Datos exportados a export.parquet")
}
