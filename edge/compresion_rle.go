package edge

import "fmt"

// CompressorRLE implementa compresión Run-Length Encoding para valores repetitivos
type CompressorRLE struct{}

func (c *CompressorRLE) Comprimir(valores []interface{}) []byte {
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

	// Almacenar número de elementos originales (4 bytes)
	numElementos := len(valoresFloat)
	resultado = append(resultado, byte(numElementos>>24))
	resultado = append(resultado, byte(numElementos>>16))
	resultado = append(resultado, byte(numElementos>>8))
	resultado = append(resultado, byte(numElementos))

	if numElementos == 0 {
		return resultado
	}

	// Aplicar RLE
	i := 0
	for i < len(valoresFloat) {
		valorActual := valoresFloat[i]
		cuenta := 1

		// Contar valores repetidos consecutivos
		for i+cuenta < len(valoresFloat) && valoresFloat[i+cuenta] == valorActual {
			cuenta++
		}

		// Almacenar valor (8 bytes) + cuenta (4 bytes)
		resultado = append(resultado, float64ToBytes(valorActual)...)

		// Dividir cuenta en chunks si es muy grande (máximo 4 bytes unsigned)
		if cuenta > 4294967295 { // máximo uint32
			cuenta = 4294967295
		}

		resultado = append(resultado, byte(cuenta>>24))
		resultado = append(resultado, byte(cuenta>>16))
		resultado = append(resultado, byte(cuenta>>8))
		resultado = append(resultado, byte(cuenta))

		i += cuenta
	}

	return resultado
}

func (c *CompressorRLE) Descomprimir(datos []byte) ([]interface{}, error) {
	if len(datos) == 0 {
		return []interface{}{}, nil
	}

	if len(datos) < 4 {
		return nil, fmt.Errorf("datos insuficientes para descomprimir RLE")
	}

	offset := 0

	// Leer número de elementos originales
	numElementos := int(datos[0])<<24 | int(datos[1])<<16 | int(datos[2])<<8 | int(datos[3])
	offset += 4

	if numElementos == 0 {
		return []interface{}{}, nil
	}

	resultado := make([]interface{}, 0, numElementos)

	// Descomprimir RLE
	for offset < len(datos) {
		// Verificar que tenemos suficientes datos para valor + cuenta
		if offset+12 > len(datos) {
			return nil, fmt.Errorf("datos insuficientes para entrada RLE")
		}

		// Leer valor (8 bytes)
		valor := bytesToFloat64(datos[offset : offset+8])
		offset += 8

		// Leer cuenta (4 bytes)
		cuenta := int(datos[offset])<<24 | int(datos[offset+1])<<16 | int(datos[offset+2])<<8 | int(datos[offset+3])
		offset += 4

		// Agregar valor repetido 'cuenta' veces
		for j := 0; j < cuenta; j++ {
			resultado = append(resultado, valor)
		}

		// Verificar que no excedamos el número original de elementos
		if len(resultado) >= numElementos {
			break
		}
	}

	// Truncar resultado al tamaño original si es necesario
	if len(resultado) > numElementos {
		resultado = resultado[:numElementos]
	}

	return resultado, nil
}
