package main

import (
	"fmt"
	"log"
    "github.com/cockroachdb/pebble"
    "github.com/cockroachdb/pebble/vfs"
)

func main() {
    opts := &pebble.Options{
        // Usar Snappy para comprimir los datos
        Levels: []pebble.LevelOptions{
            {Compression: pebble.SnappyCompression},
        },
        // Usar el sistema de archivos por defecto
        FS: vfs.Default,
    }
    
    // Abrir la base de datos
    db, err := pebble.Open("sensor.db", opts)
    // manejo del error
    if err != nil {
		log.Fatalf("Error al abrir la base de datos: %v", err)
    }
    // Cerrar la base de datos al finalizar
    defer db.Close()
    
    // Escribir la clave "Hello" con el valor "World"
	db.Set([]byte("Hello"), []byte("World"), pebble.Sync)

    // Leer el valor de la clave "Hello"
	valor, closer, err := db.Get([]byte("Hello"))
    // manejo del error
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Valor %s\n", valor)
    // Cerrar el cursor
    closer.Close()
}