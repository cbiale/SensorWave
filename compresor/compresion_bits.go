/*
## Algoritmo de Compresión Bits - Para enteros en rangos pequeños

Objetivo: Comprimir enteros que se encuentran en un rango limitado usando
el mínimo número de bits necesarios.

Entrada: Array de valores enteros [v₀, v₁, v₂, ..., vₙ]

Salida: Datos comprimidos con formato:
• count: número de valores (4 bytes uint32)
• min: valor mínimo (8 bytes int64)
• max: valor máximo (8 bytes int64)
• bits_per_value: bits necesarios por valor (1 byte uint8, rango 0-64)
• packed_data: valores empaquetados en bits

Algoritmo:

1. Calcular rango:
 • min ← valor mínimo del array
 • max ← valor máximo del array
 • range ← max - min

2. Calcular bits necesarios:
 • Si range == 0: bits = 0 (todos los valores son iguales)
 • Sino: bits = ceil(log2(range + 1))
 • Máximo 64 bits

3. Empaquetar valores:
 • Para cada valor v:
   • normalized ← v - min
   • Escribir normalized usando bits_per_value bits

4. Formato final:
 • [count: 4 bytes][min: 8 bytes][max: 8 bytes][bits_per_value: 1 byte][packed_data]

Complejidad: O(n) tiempo, O(1) espacio adicional

Casos óptimos:
• Valores en rango 0-255: 8 bits por valor
• Valores en rango 0-15: 4 bits por valor
• Valores todos iguales: 0 bits por valor (solo header)
*/

package compresor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
)

// CompresorBitsGenerico implementa compresión por bits para enteros
// NOTA: Aunque el constraint Numeric permite int64 y float64, este compresor
// solo tiene sentido para valores enteros (int64) ya que optimiza rangos discretos.
// El sistema normaliza todos los enteros a int64 antes de la compresión.
type CompresorBitsGenerico[T Numeric] struct{}

// Comprimir comprime valores enteros usando el mínimo de bits necesarios
func (c *CompresorBitsGenerico[T]) Comprimir(valores []T) ([]byte, error) {
	if len(valores) == 0 {
		return []byte{}, nil
	}

	// Convertir a int64 para cálculos
	valoresInt := make([]int64, len(valores))
	for i, v := range valores {
		valoresInt[i] = toInt64(v)
	}

	// Calcular min y max
	min := valoresInt[0]
	max := valoresInt[0]
	for _, v := range valoresInt {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Calcular bits necesarios
	rangeVal := max - min
	var bitsNecesarios uint8

	if rangeVal == 0 {
		// Todos los valores son iguales
		bitsNecesarios = 0
	} else {
		// Calcular bits necesarios: ceil(log2(range + 1))
		bitsNecesarios = uint8(64 - bits.LeadingZeros64(uint64(rangeVal)))
	}

	// Serializar header
	var buffer bytes.Buffer
	// Escribir count de valores (4 bytes)
	binary.Write(&buffer, binary.LittleEndian, uint32(len(valores)))
	binary.Write(&buffer, binary.LittleEndian, min)
	binary.Write(&buffer, binary.LittleEndian, max)
	buffer.WriteByte(bitsNecesarios)

	// Si todos los valores son iguales, no necesitamos datos
	if bitsNecesarios == 0 {
		return buffer.Bytes(), nil
	}

	// Empaquetar valores
	writer := newBitWriter()
	for _, v := range valoresInt {
		normalized := uint64(v - min)
		writer.writeBits(normalized, int(bitsNecesarios))
	}

	// Agregar datos empaquetados
	buffer.Write(writer.getBytes())

	return buffer.Bytes(), nil
}

// Descomprimir descomprime valores empaquetados en bits
func (c *CompresorBitsGenerico[T]) Descomprimir(datos []byte) ([]T, error) {
	if len(datos) == 0 {
		return []T{}, nil
	}

	reader := bytes.NewReader(datos)

	// Leer header
	var count uint32
	var min, max int64
	var bitsNecesarios uint8

	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return nil, fmt.Errorf("error leyendo count: %v", err)
	}
	if err := binary.Read(reader, binary.LittleEndian, &min); err != nil {
		return nil, fmt.Errorf("error leyendo min: %v", err)
	}
	if err := binary.Read(reader, binary.LittleEndian, &max); err != nil {
		return nil, fmt.Errorf("error leyendo max: %v", err)
	}
	if err := binary.Read(reader, binary.LittleEndian, &bitsNecesarios); err != nil {
		return nil, fmt.Errorf("error leyendo bits_per_value: %v", err)
	}

	// Si bits es 0, todos los valores son iguales al min
	if bitsNecesarios == 0 {
		resultado := make([]T, count)
		for i := uint32(0); i < count; i++ {
			resultado[i] = fromInt64[T](min)
		}
		return resultado, nil
	}

	// Leer datos empaquetados
	packedData := make([]byte, reader.Len())
	reader.Read(packedData)

	bitReader := newBitReader(packedData)
	resultado := make([]T, 0, count)

	// Leer exactamente count valores
	for i := uint32(0); i < count; i++ {
		normalized, err := bitReader.readBits(int(bitsNecesarios))
		if err != nil {
			return nil, fmt.Errorf("error leyendo valor %d: %v", i, err)
		}

		valor := int64(normalized) + min
		resultado = append(resultado, fromInt64[T](valor))
	}

	return resultado, nil
}
