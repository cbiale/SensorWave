package compresor

import (
	"fmt"
	"math"
	"math/bits"
)

// CompresorXor implementa compresión XOR (Gorilla) para valores float64
// IMPORTANTE: Este compresor NO es genérico y solo soporta float64.
// El algoritmo XOR/Gorilla está específicamente diseñado para aprovechar
// la representación binaria IEEE 754 de números de punto flotante.
//
// NORMALIZACIÓN: El sistema convierte automáticamente float32 a float64
// antes de la compresión. Ver compresor/compresion_utils.go:ConvertirAFloat64Array()
type CompresorXor struct{}

// bitWriter maneja la escritura de bits individuales
type bitWriter struct {
	bytes []byte
	count int // número de bits escritos en el byte actual
}

// newBitWriter crea un nuevo bitWriter
func newBitWriter() *bitWriter {
	return &bitWriter{
		bytes: make([]byte, 0),
		count: 0,
	}
}

// writeBit escribe un solo bit
func (bw *bitWriter) writeBit(bit bool) {
	if bw.count == 0 {
		bw.bytes = append(bw.bytes, 0)
	}

	if bit {
		bw.bytes[len(bw.bytes)-1] |= 1 << (7 - bw.count)
	}

	bw.count++
	if bw.count == 8 {
		bw.count = 0
	}
}

// writeBits escribe múltiples bits
func (bw *bitWriter) writeBits(value uint64, numBits int) {
	for i := numBits - 1; i >= 0; i-- {
		bw.writeBit((value & (1 << i)) != 0)
	}
}

// getBytes obtiene los bytes escritos
func (bw *bitWriter) getBytes() []byte {
	return bw.bytes
}

// Comprimir comprime una serie de valores float64 usando el algoritmo Xor
func (c *CompresorXor) Comprimir(valores []float64) ([]byte, error) {
	// si el largo es cero retorno arreglo vacio
	if len(valores) == 0 {
		return []byte{}, nil
	}

	writer := newBitWriter()

	// Escribir el número de valores (32 bits)
	writer.writeBits(uint64(len(valores)), 32)

	// (1) The first value is stored uncompressed using 64 bits.
	primerValor := math.Float64bits(valores[0])
	writer.writeBits(primerValor, 64)

	// Variables para mantener el estado de la compresión
	valorAnterior := primerValor
	leadingZerosAnterior := 0
	trailingZerosAnterior := 0

	// Procesar el resto de los valores
	for i := 1; i < len(valores); i++ {
		valorActual := math.Float64bits(valores[i])
		xor := valorActual ^ valorAnterior

		// (2) If the XOR result is zero (i.e., the value is identical to the previous record),
		// store '0' in 1 bit.
		if xor == 0 {
			writer.writeBit(false)
			continue
		}

		// (3) If the XOR result is non-zero, compute the number of leading
		// and trailing zeros, then store '1' in 1 bit
		writer.writeBit(true)

		leadingZeros := bits.LeadingZeros64(xor)
		trailingZeros := bits.TrailingZeros64(xor)

		// Calcular los bits significativos
		meaningfulBits := 64 - leadingZeros - trailingZeros

		// (a) If the meaningful bits fit within the length of the previously stored meaningful bits,
		// store '0' in 1 bit, followed by meaningful bits, using the same encoding as the previous value.
		if leadingZeros >= leadingZerosAnterior &&
			trailingZeros >= trailingZerosAnterior &&
			leadingZerosAnterior != 0 { // Asegurarse de que no es el primer XOR no-cero

			writer.writeBit(false)

			// Extraer los bits significativos usando el rango anterior
			meaningfulBitsAnterior := 64 - leadingZerosAnterior - trailingZerosAnterior
			significantValue := (xor >> trailingZerosAnterior) & ((1 << meaningfulBitsAnterior) - 1)
			writer.writeBits(significantValue, meaningfulBitsAnterior)
		} else {
			// (b) Otherwise, store '1' in 1 bit. Store the number of leading zeros in the next 5 bits,
			// the length of meaningful bits in the next 6 bits, and the meaningful bits.
			writer.writeBit(true)

			// Almacenar leading zeros (5 bits, permite valores 0-31)
			writer.writeBits(uint64(leadingZeros), 5)

			// Almacenar longitud de bits significativos (6 bits, permite valores 0-63)
			writer.writeBits(uint64(meaningfulBits), 6)

			// Almacenar los bits significativos
			significantValue := (xor >> trailingZeros) & ((1 << meaningfulBits) - 1)
			writer.writeBits(significantValue, meaningfulBits)

			// Actualizar los valores anteriores
			leadingZerosAnterior = leadingZeros
			trailingZerosAnterior = trailingZeros
		}

		valorAnterior = valorActual
	}

	return writer.getBytes(), nil
}

// bitReader maneja la lectura de bits individuales
type bitReader struct {
	bytes []byte
	pos   int // posición actual en bits
}

// newBitReader crea un nuevo bitReader
func newBitReader(data []byte) *bitReader {
	return &bitReader{
		bytes: data,
		pos:   0,
	}
}

// readBit lee un solo bit
func (br *bitReader) readBit() (bool, error) {
	if br.pos/8 >= len(br.bytes) {
		return false, fmt.Errorf("fin de datos alcanzado")
	}

	byteIdx := br.pos / 8
	bitIdx := 7 - (br.pos % 8)
	bit := (br.bytes[byteIdx] & (1 << bitIdx)) != 0
	br.pos++

	return bit, nil
}

// readBits lee múltiples bits
func (br *bitReader) readBits(numBits int) (uint64, error) {
	var result uint64
	for i := 0; i < numBits; i++ {
		bit, err := br.readBit()
		if err != nil {
			return 0, err
		}
		result <<= 1
		if bit {
			result |= 1
		}
	}
	return result, nil
}

// Descomprimir descomprime una serie de valores float64 usando el algoritmo Xor
func (c *CompresorXor) Descomprimir(datos []byte) ([]float64, error) {
	if len(datos) == 0 {
		return []float64{}, nil
	}

	reader := newBitReader(datos)
	valores := make([]float64, 0)

	// Leer el número de valores (32 bits)
	numValores, err := reader.readBits(32)
	if err != nil {
		return nil, fmt.Errorf("error leyendo número de valores: %v", err)
	}

	if numValores == 0 {
		return []float64{}, nil
	}

	// Leer el primer valor (64 bits sin comprimir)
	primerValor, err := reader.readBits(64)
	if err != nil {
		return nil, fmt.Errorf("error leyendo primer valor: %v", err)
	}
	valores = append(valores, math.Float64frombits(primerValor))

	valorAnterior := primerValor
	leadingZerosAnterior := 0
	trailingZerosAnterior := 0

	// Leer el resto de los valores hasta alcanzar numValores
	for len(valores) < int(numValores) {
		bit, err := reader.readBit()
		if err != nil {
			return nil, fmt.Errorf("error leyendo bit de control: %v", err)
		}

		if !bit {
			// El valor es idéntico al anterior
			valores = append(valores, math.Float64frombits(valorAnterior))
			continue
		}

		// Leer siguiente bit para determinar si usa encoding anterior
		bit, err = reader.readBit()
		if err != nil {
			return nil, fmt.Errorf("error leyendo bit de encoding: %v", err)
		}

		var xor uint64

		if !bit {
			// Usar encoding anterior
			meaningfulBits := 64 - leadingZerosAnterior - trailingZerosAnterior
			significantValue, err := reader.readBits(meaningfulBits)
			if err != nil {
				return nil, fmt.Errorf("error leyendo bits significativos: %v", err)
			}
			xor = significantValue << trailingZerosAnterior
		} else {
			// Nuevo encoding
			leadingZeros, err := reader.readBits(5)
			if err != nil {
				return nil, fmt.Errorf("error leyendo leading zeros: %v", err)
			}

			meaningfulBits, err := reader.readBits(6)
			if err != nil {
				return nil, fmt.Errorf("error leyendo meaningful bits: %v", err)
			}

			significantValue, err := reader.readBits(int(meaningfulBits))
			if err != nil {
				return nil, fmt.Errorf("error leyendo valor significativo: %v", err)
			}

			trailingZeros := 64 - int(leadingZeros) - int(meaningfulBits)
			xor = significantValue << trailingZeros

			leadingZerosAnterior = int(leadingZeros)
			trailingZerosAnterior = trailingZeros
		}

		valorActual := valorAnterior ^ xor
		valores = append(valores, math.Float64frombits(valorActual))
		valorAnterior = valorActual
	}

	return valores, nil
}
