package compresor

import (
	"bytes"
	"github.com/klauspost/compress/gzip"
	"fmt"
	"io"
)

// CompresorGzip implementa compresión Gzip
type CompresorGzip struct{}

// Comprimir comprime los datos usando Gzip
func (c *CompresorGzip) Comprimir(datos []byte) ([]byte, error) {
	// Si no hay datos, retornar vacío
	if len(datos) == 0 {
		return []byte{}, nil
	}

	// Buffer para almacenar datos comprimidos
	var buf bytes.Buffer

	// Crear writer de gzip
	gzipWriter := gzip.NewWriter(&buf)

	// Escribir datos originales
	_, err := gzipWriter.Write(datos)
	if err != nil {
		return []byte{}, fmt.Errorf("error al escribir datos gzip: %v", err)
	}

	// Cerrar writer para forzar el flush
	err = gzipWriter.Close()
	if err != nil {
		return []byte{}, fmt.Errorf("error al cerrar writer gzip: %v", err)
	}

	// Retornar datos comprimidos
	return buf.Bytes(), nil
}

// Descomprimir descomprime los datos usando Gzip
func (c *CompresorGzip) Descomprimir(datos []byte) ([]byte, error) {
	// Si no hay datos, retornar vacío
	if len(datos) == 0 {
		return []byte{}, nil
	}

	// Crear reader de gzip
    descomprimido, err := gzip.NewReader(bytes.NewReader(datos))
	if err != nil {
		return []byte{}, fmt.Errorf("error al crear reader gzip: %v", err)
	}
	defer descomprimido.Close()
	
	// Leer datos descomprimidos
	resultado, err := io.ReadAll(descomprimido)
	if err != nil {
		return []byte{}, fmt.Errorf("error al leer datos gzip: %v", err)
	}

	// Retornar datos descomprimidos
	return resultado, nil
}
