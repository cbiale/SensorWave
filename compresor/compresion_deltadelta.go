package compresor

import "fmt"

// CompressorDeltaDeltaValores implementa compresión DeltaDelta para valores
type CompressorDeltaDeltaValores struct{}

func (c *CompressorDeltaDeltaValores) Comprimir(valores []interface{}) []byte {
	// si el largo es cero retorno arreglo vacio
	if len(valores) == 0 {
		return []byte{}
	}

	// Creo un arreglo de resultados
	resultado := make([]byte, 0)

	// Convertir todos los valores a float64 y validar
	valoresFloat := make([]float64, len(valores))
	for i, valor := range valores {
		if v, ok := valor.(float64); ok {
			valoresFloat[i] = v
		} else {
			// Si no es float64, usar 0.0
			valoresFloat[i] = 0.0
		}
	}

	// Almacenar número de elementos (4 bytes)
	numElementos := len(valoresFloat)
	resultado = append(resultado, byte(numElementos>>24))
	resultado = append(resultado, byte(numElementos>>16))
	resultado = append(resultado, byte(numElementos>>8))
	resultado = append(resultado, byte(numElementos))

	// Almacenar el primer valor sin compresión (8 bytes)
	resultado = append(resultado, float64ToBytes(valoresFloat[0])...)

	if len(valoresFloat) == 1 {
		return resultado
	}

	// Almacenar primera delta (4 bytes como float32 para ahorrar espacio)
	var deltaValorAnterior float64
	if len(valoresFloat) > 1 {
		deltaValorAnterior = valoresFloat[1] - valoresFloat[0]
		resultado = append(resultado, float32ToBytes(float32(deltaValorAnterior))...)
	}

	if len(valoresFloat) == 2 {
		return resultado
	}

	// Aplicar compresión DeltaDelta para el resto de elementos (desde el índice 2)
	for i := 2; i < len(valoresFloat); i++ {
		valorActual := valoresFloat[i]

		// Calcular delta de valor
		deltaValor := valorActual - valoresFloat[i-1]
		deltaDeltaValor := deltaValor - deltaValorAnterior

		// Determinar cuántos bytes necesitamos para la delta-delta
		var deltaBytes []byte
		deltaDeltaFloat32 := float32(deltaDeltaValor)

		// Para simplificar, siempre usar 4 bytes para delta-delta de valores
		deltaBytes = append(deltaBytes, float32ToBytes(deltaDeltaFloat32)...)

		resultado = append(resultado, deltaBytes...)

		// Actualizar delta anterior
		deltaValorAnterior = deltaValor
	}

	return resultado
}

func (c *CompressorDeltaDeltaValores) Descomprimir(datos []byte) ([]interface{}, error) {
	if len(datos) == 0 {
		return []interface{}{}, nil
	}

	if len(datos) < 4 {
		return nil, fmt.Errorf("datos insuficientes para descomprimir valores DeltaDelta")
	}

	offset := 0

	// Leer número de elementos
	numElementos := int(datos[0])<<24 | int(datos[1])<<16 | int(datos[2])<<8 | int(datos[3])
	offset += 4

	if numElementos == 0 {
		return []interface{}{}, nil
	}

	// Leer primer valor (8 bytes)
	if offset+8 > len(datos) {
		return nil, fmt.Errorf("datos insuficientes para primer valor")
	}

	resultado := make([]interface{}, 0, numElementos)
	primerValor := bytesToFloat64(datos[offset : offset+8])
	resultado = append(resultado, primerValor)
	offset += 8

	if numElementos == 1 {
		return resultado, nil
	}

	// Leer primera delta (4 bytes)
	if offset+4 > len(datos) {
		return nil, fmt.Errorf("datos insuficientes para primera delta")
	}

	deltaValorAnterior := float64(bytesToFloat32(datos[offset : offset+4]))
	segundoValor := primerValor + deltaValorAnterior
	resultado = append(resultado, segundoValor)
	offset += 4

	if numElementos == 2 {
		return resultado, nil
	}

	// Descomprimir el resto usando delta-delta
	valorAnterior := segundoValor

	for i := 2; i < numElementos; i++ {
		if offset+4 > len(datos) {
			return nil, fmt.Errorf("datos insuficientes para delta-delta en posición %d", i)
		}

		// Leer delta-delta (4 bytes)
		deltaDeltaValor := float64(bytesToFloat32(datos[offset : offset+4]))
		offset += 4

		// Reconstruir el valor
		deltaValor := deltaValorAnterior + deltaDeltaValor
		valorActual := valorAnterior + deltaValor

		resultado = append(resultado, valorActual)

		// Actualizar para siguiente iteración
		deltaValorAnterior = deltaValor
		valorAnterior = valorActual
	}

	return resultado, nil
}
