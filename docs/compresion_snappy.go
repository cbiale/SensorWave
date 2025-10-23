package edge

import (
	"fmt"
	"github.com/klauspost/compress/snappy"
)


type CompressorSnappy struct{}


func (c *CompressorSnappy) Comprimir(datos []byte) ([]byte, error) {

	if len(datos) == 0 {
		return []byte{}, nil
	}

	comprimido := snappy.Encode(nil, datos)
	
	return comprimido, nil
}

func (c *CompressorSnappy) Descomprimir(datos []byte) ([]byte, error) {
	if len(datos) == 0 {
		return []byte{}, nil
	}

	descomprimido, err := snappy.Decode(nil, datos)
	if err != nil {
		return []byte{}, fmt.Errorf("error al descomprimir con Snappy: %v", err)
	}

	return descomprimido, nil
}


