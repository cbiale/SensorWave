package compresor

import 
(
	"fmt"
	"github.com/cbiale/sensorwave/tipos"
)

// CompressorValor define la interfaz para compresores de valores (nivel 1)
type CompressorValor interface {
	Comprimir(valores []interface{}) []byte
	Descomprimir(datos []byte) ([]interface{}, error)
}

// obtenerCompressorValor factory para crear compresores de valores
func ObtenerCompressorValor(tipo tipos.TipoCompresionValores) CompressorValor {
	switch tipo {
	case tipos.DeltaDelta:
		return &CompressorDeltaDeltaValores{}
	case tipos.RLE:
		return &CompressorRLE{}
	case tipos.Bits:
		return &CompressorBits{}
	case tipos.SinCompresion:
		return &CompressorValoresNinguno{}
	default:
		return &CompressorValoresNinguno{}
	}
}

// CompressorValoresNinguno implementa sin compresión para valores
type CompressorValoresNinguno struct{}

func (c *CompressorValoresNinguno) Comprimir(valores []interface{}) []byte {
	if len(valores) == 0 {
		return []byte{}
	}

	resultado := make([]byte, 0)

	// Agregar header con número de elementos
	numElementos := len(valores)
	resultado = append(resultado, byte(numElementos>>24))
	resultado = append(resultado, byte(numElementos>>16))
	resultado = append(resultado, byte(numElementos>>8))
	resultado = append(resultado, byte(numElementos))

	// Serializar cada valor como float64 (8 bytes)
	for _, valor := range valores {
		if v, ok := valor.(float64); ok {
			valorBytes := float64ToBytes(v)
			resultado = append(resultado, valorBytes...)
		} else {
			// Si no es float64, convertir a 0.0
			valorBytes := float64ToBytes(0.0)
			resultado = append(resultado, valorBytes...)
		}
	}

	return resultado
}

func (c *CompressorValoresNinguno) Descomprimir(datos []byte) ([]interface{}, error) {
	if len(datos) < 4 {
		return nil, fmt.Errorf("datos insuficientes para descomprimir")
	}

	// Leer número de elementos
	numElementos := int(datos[0])<<24 | int(datos[1])<<16 | int(datos[2])<<8 | int(datos[3])
	datos = datos[4:]

	if len(datos) < numElementos*8 {
		return nil, fmt.Errorf("datos insuficientes para el número de elementos especificado")
	}

	resultado := make([]interface{}, numElementos)
	for i := 0; i < numElementos; i++ {
		inicio := i * 8
		valorBytes := datos[inicio : inicio+8]
		valor := bytesToFloat64(valorBytes)
		resultado[i] = valor
	}

	return resultado, nil
}
