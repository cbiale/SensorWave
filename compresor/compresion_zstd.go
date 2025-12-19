package compresor

import (
	"fmt"
	"github.com/klauspost/compress/zstd"
)

// CompresorZSTD implementa compresión Zstd
type CompresorZSTD struct{}

// Comprimir comprime los datos usando el algoritmo Zstd.
func (c *CompresorZSTD) Comprimir(datos []byte) ([]byte, error) {
	// Si no hay datos, retornar vacío
	if len(datos) == 0 {
		return []byte{}, nil
	}

	// Crear un nuevo encoder Zstd
    encoder, err := zstd.NewWriter(nil)
    if err != nil {
		return []byte{}, fmt.Errorf("error al crear encoder con Zstd: %v", err)
    }
    defer encoder.Close()
    
	// Comprimir los datos
    comprimido := encoder.EncodeAll(datos, make([]byte, 0, len(datos)))

	// Retornar datos comprimidos
	return comprimido, nil    
}

// Descomprimir descomprime los datos usando el algoritmo Zstd.
func (c *CompresorZSTD) Descomprimir(datos []byte) ([]byte, error) {

	// Si no hay datos, retornar vacío
	if len(datos) == 0 {
		return []byte{}, nil
	}

	// Crear un nuevo decoder Zstd
    decoder, err := zstd.NewReader(nil)
    if err != nil {
        return []byte{}, fmt.Errorf("error creando decoder co Zstd: %v", err)
    }
    defer decoder.Close()
    
	// Descomprimir los datos
    descomprimido, err := decoder.DecodeAll(datos, []byte{})
	if err != nil {
		return []byte{}, fmt.Errorf("error al descomprimir con Zstd: %v", err)
	}

	// Retornar datos descomprimidos
	return descomprimido, nil   
}


