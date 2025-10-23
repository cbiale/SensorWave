package compresor

import (
	"github.com/cbiale/sensorwave/tipos"
)

// CompressorBloque define la interfaz para compresores de bloques (nivel 2)
type CompressorBloque interface {
	Comprimir(datos []byte) ([]byte, error)
	Descomprimir(datos []byte) ([]byte, error)
}

// obtenerCompressorBloque factory para crear compresores de bloques
func ObtenerCompressorBloque(tipo tipos.TipoCompresionBloque) CompressorBloque {
	switch tipo {
	case tipos.LZ4:
		return &CompressorLZ4{}
	case tipos.ZSTD:
		return &CompressorZSTD{}
	case tipos.Snappy:
		return &CompressorSnappy{}
	case tipos.Gzip:
		return &CompressorGzip{}
	case tipos.Ninguna:
		return &CompressorBloqueNinguno{}
	default:
		return &CompressorBloqueNinguno{}
	}
}

// obtenerCompressorGzip devuelve el compresor Gzip (como alternativa)
func obtenerCompressorGzip() CompressorBloque {
	return &CompressorGzip{}
}

// CompressorBloqueNinguno implementa sin compresión para bloques
type CompressorBloqueNinguno struct{}

func (c *CompressorBloqueNinguno) Comprimir(datos []byte) ([]byte, error) {
	return datos, nil
}

func (c *CompressorBloqueNinguno) Descomprimir(datos []byte) ([]byte, error) {
	return datos, nil
}
