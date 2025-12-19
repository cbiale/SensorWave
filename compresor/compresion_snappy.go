package compresor

import (
	"fmt"
	"github.com/klauspost/compress/snappy"
)

// CompresorSnappy implementa compresión Snappy
type CompresorSnappy struct{}

// Comprimir comprime los datos usando Snappy
func (c *CompresorSnappy) Comprimir(datos []byte) ([]byte, error) {
	// Si no hay datos, retornar vacío
	if len(datos) == 0 {
		return []byte{}, nil
	}

	// Comprimir usando Snappy
	comprimido := snappy.Encode(nil, datos)
	
	// Retornar datos comprimidos
	return comprimido, nil
}

// Descomprimir descomprime los datos usando Snappy
func (c *CompresorSnappy) Descomprimir(datos []byte) ([]byte, error) {
	// Si no hay datos, retornar vacío
	if len(datos) == 0 {
		return []byte{}, nil
	}

	// Descomprimir usando Snappy
	descomprimido, err := snappy.Decode(nil, datos)
	if err != nil {
		return []byte{}, fmt.Errorf("error al descomprimir con Snappy: %v", err)
	}

	// Retornar datos descomprimidos
	return descomprimido, nil
}