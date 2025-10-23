package compresor

import (
	"fmt"
	"math"
)

// CompressorBits implementa compresión por bits para valores que pueden representarse con menos bits
type CompressorBits struct{}

func (c *CompressorBits) Comprimir(valores []interface{}) []byte {
	if len(valores) == 0 {
		return []byte{}
	}

	resultado := make([]byte, 0)

	// Convertir todos los valores a float64
	valoresFloat := make([]float64, len(valores))
	for i, valor := range valores {
		if v, ok := valor.(float64); ok {
			valoresFloat[i] = v
		} else {
			valoresFloat[i] = 0.0
		}
	}

	// Almacenar número de elementos (4 bytes)
	numElementos := len(valoresFloat)
	resultado = append(resultado, byte(numElementos>>24))
	resultado = append(resultado, byte(numElementos>>16))
	resultado = append(resultado, byte(numElementos>>8))
	resultado = append(resultado, byte(numElementos))

	if numElementos == 0 {
		return resultado
	}

	// Analizar los valores para determinar si son enteros pequeños
	sonEnteros := true
	valorMin := valoresFloat[0]
	valorMax := valoresFloat[0]

	for _, valor := range valoresFloat {
		// Verificar si es un entero
		if valor != float64(int64(valor)) {
			sonEnteros = false
		}
		if valor < valorMin {
			valorMin = valor
		}
		if valor > valorMax {
			valorMax = valor
		}
	}

	// Si son enteros pequeños, aplicar compresión por bits
	if sonEnteros && valorMin >= 0 && valorMax <= 255 {
		// Usar 1 byte por valor
		resultado = append(resultado, 0x01) // Flag: 1 byte por valor
		for _, valor := range valoresFloat {
			resultado = append(resultado, byte(int64(valor)))
		}
	} else if sonEnteros && valorMin >= -32768 && valorMax <= 32767 {
		// Usar 2 bytes por valor
		resultado = append(resultado, 0x02) // Flag: 2 bytes por valor
		for _, valor := range valoresFloat {
			valorInt := int16(valor)
			resultado = append(resultado, byte(valorInt>>8))
			resultado = append(resultado, byte(valorInt))
		}
	} else if sonEnteros && valorMin >= -2147483648 && valorMax <= 2147483647 {
		// Usar 4 bytes por valor
		resultado = append(resultado, 0x04) // Flag: 4 bytes por valor
		for _, valor := range valoresFloat {
			valorInt := int32(valor)
			resultado = append(resultado, int32ToBytes(valorInt)...)
		}
	} else {
		// Usar float32 (4 bytes) en lugar de float64 (8 bytes) para ahorrar espacio
		resultado = append(resultado, 0x08) // Flag: 4 bytes float32
		for _, valor := range valoresFloat {
			valorFloat32 := float32(valor)
			resultado = append(resultado, float32ToBytes(valorFloat32)...)
		}
	}

	return resultado
}

func (c *CompressorBits) Descomprimir(datos []byte) ([]interface{}, error) {
	if len(datos) == 0 {
		return []interface{}{}, nil
	}

	if len(datos) < 5 { // 4 bytes para numElementos + 1 byte para flag
		return nil, fmt.Errorf("datos insuficientes para descomprimir Bits")
	}

	offset := 0

	// Leer número de elementos
	numElementos := int(datos[0])<<24 | int(datos[1])<<16 | int(datos[2])<<8 | int(datos[3])
	offset += 4

	if numElementos == 0 {
		return []interface{}{}, nil
	}

	// Leer flag de tipo de compresión
	flag := datos[offset]
	offset++

	resultado := make([]interface{}, 0, numElementos)

	switch flag {
	case 0x01: // 1 byte por valor
		if offset+numElementos > len(datos) {
			return nil, fmt.Errorf("datos insuficientes para valores de 1 byte")
		}
		for i := 0; i < numElementos; i++ {
			valor := float64(datos[offset+i])
			resultado = append(resultado, valor)
		}

	case 0x02: // 2 bytes por valor
		if offset+numElementos*2 > len(datos) {
			return nil, fmt.Errorf("datos insuficientes para valores de 2 bytes")
		}
		for i := 0; i < numElementos; i++ {
			valorInt := int16(datos[offset])<<8 | int16(datos[offset+1])
			valor := float64(valorInt)
			resultado = append(resultado, valor)
			offset += 2
		}

	case 0x04: // 4 bytes entero por valor
		if offset+numElementos*4 > len(datos) {
			return nil, fmt.Errorf("datos insuficientes para valores de 4 bytes")
		}
		for i := 0; i < numElementos; i++ {
			valorInt := bytesToInt32(datos[offset : offset+4])
			valor := float64(valorInt)
			resultado = append(resultado, valor)
			offset += 4
		}

	case 0x08: // 4 bytes float32 por valor
		if offset+numElementos*4 > len(datos) {
			return nil, fmt.Errorf("datos insuficientes para valores float32")
		}
		for i := 0; i < numElementos; i++ {
			valorFloat32 := bytesToFloat32(datos[offset : offset+4])
			// Verificar NaN o Inf
			if math.IsNaN(float64(valorFloat32)) || math.IsInf(float64(valorFloat32), 0) {
				resultado = append(resultado, 0.0)
			} else {
				resultado = append(resultado, float64(valorFloat32))
			}
			offset += 4
		}

	default:
		return nil, fmt.Errorf("flag de compresión desconocido: %x", flag)
	}

	return resultado, nil
}
