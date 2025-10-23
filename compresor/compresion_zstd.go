package compresor

import (
	"fmt"
	"github.com/klauspost/compress/zstd"
)


type CompressorZSTD struct{}


func (c *CompressorZSTD) Comprimir(datos []byte) ([]byte, error) {

	if len(datos) == 0 {
		return []byte{}, nil
	}

    encoder, err := zstd.NewWriter(nil)
    if err != nil {
        fmt.Print("error creando encoder con Zstd:", err)
		return []byte{}, err
    }
    defer encoder.Close()
    
    comprimido := encoder.EncodeAll(datos, make([]byte, 0, len(datos)))
	return comprimido, nil    
}

func (c *CompressorZSTD) Descomprimir(datos []byte) ([]byte, error) {
	if len(datos) == 0 {
		return []byte{}, nil
	}

    decoder, err := zstd.NewReader(nil)
    if err != nil {
        return []byte{}, fmt.Errorf("error creando decoder co Zstd: %v", err)
    }
    defer decoder.Close()
    
    descomprimido, err := decoder.DecodeAll(datos, []byte{})
	if err != nil {
		return []byte{}, fmt.Errorf("error al descomprimir con Zstd: %v", err)
	}
	return descomprimido, nil   
}


