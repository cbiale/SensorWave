package compresor

import (
	"bytes"
	"fmt"
	"io"

	"github.com/pierrec/lz4/v4"
)

// CompresorLZ4 implementa compresión LZ4
type CompresorLZ4 struct{}

// Comprimir comprime los datos usando LZ4
func (c *CompresorLZ4) Comprimir(datos []byte) ([]byte, error) {
	if len(datos) == 0 {
		return []byte{}, nil
	}

	var comprimido bytes.Buffer
	writer := lz4.NewWriter(&comprimido)

	_, err := writer.Write(datos)
	if err != nil {
		return []byte{}, fmt.Errorf("error al escribir datos LZ4: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return []byte{}, fmt.Errorf("error al cerrar writer LZ4: %v", err)
	}

	return comprimido.Bytes(), nil
}

func (c *CompresorLZ4) Descomprimir(datos []byte) ([]byte, error) {
	if len(datos) == 0 {
		return []byte{}, nil
	}

	reader := lz4.NewReader(bytes.NewReader(datos))
	resultado, err := io.ReadAll(reader)
	if err != nil {
		return []byte{}, fmt.Errorf("error al descomprimir con LZ4: %v", err)
	}

	return resultado, nil
}