package compresor

import (
	"bytes"
	"github.com/klauspost/compress/gzip"
	"fmt"
	"io"
)

// CompressorGzip implementa compresión Gzip para bloques usando la biblioteca estándar
type CompressorGzip struct{}

func (c *CompressorGzip) Comprimir(datos []byte) ([]byte, error) {
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

	return buf.Bytes(), nil
}

func (c *CompressorGzip) Descomprimir(datos []byte) ([]byte, error) {
	if len(datos) == 0 {
		return []byte{}, nil
	}

    descomprimido, err := gzip.NewReader(bytes.NewReader(datos))
	if err != nil {
		return []byte{}, fmt.Errorf("error al crear reader gzip: %v", err)
	}
	defer descomprimido.Close()
	
	resultado, err := io.ReadAll(descomprimido)
	if err != nil {
		return []byte{}, fmt.Errorf("error al leer datos gzip: %v", err)
	}

	return resultado, nil
}
