package edge

// CompressorBloque define la interfaz para compresores de bloques (nivel 2)
type CompressorBloque interface {
	Comprimir(datos []byte) ([]byte, error)
	Descomprimir(datos []byte) ([]byte, error)
}

// obtenerCompressorBloque factory para crear compresores de bloques
func (me *ManagerEdge) obtenerCompressorBloque(tipo TipoCompresionBloque) CompressorBloque {
	switch tipo {
	case LZ4:
		return &CompressorLZ4{}
	case ZSTD:
		return &CompressorZSTD{}
	case Snappy:
		return &CompressorSnappy{}
	case Gzip:
		return &CompressorGzip{}
	case Ninguna:
		return &CompressorBloqueNinguno{}
	default:
		return &CompressorBloqueNinguno{}
	}
}

// obtenerCompressorGzip devuelve el compresor Gzip (como alternativa)
func (me *ManagerEdge) obtenerCompressorGzip() CompressorBloque {
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
