package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/bits"
	"strconv"
	"time"

	"github.com/tecbot/gorocksdb"
)

// SensorDataCompressor gestiona la compresión y almacenamiento de datos de sensores
type SensorDataCompressor struct {
	db           *gorocksdb.DB
	readOptions  *gorocksdb.ReadOptions
	writeOptions *gorocksdb.WriteOptions
	prevValues   map[string]float64
}

// CompressionType define los tipos de compresión disponibles
type CompressionType string

const (
	CompressGorilla    CompressionType = "gorilla"
	CompressDelta      CompressionType = "delta"
	CompressDeltaDelta CompressionType = "delta-delta"
	CompressRLE        CompressionType = "rle"
)

// CompressedBlock representa un bloque de datos comprimidos
type CompressedBlock struct {
	Compression CompressionType `json:"compression"`
	StartTime   int64           `json:"start_time"`
	EndTime     int64           `json:"end_time"`
	Count       int             `json:"count"`
	Data        []byte          `json:"data,omitempty"`
	RLEData     []RLEEntry      `json:"rle_data,omitempty"`
}

// RLEEntry representa una entrada en la codificación RLE
type RLEEntry struct {
	Value float64 `json:"value"`
	Count int     `json:"count"`
}

// NewSensorDataCompressor crea una nueva instancia del compresor
func NewSensorDataCompressor(dbPath string) (*SensorDataCompressor, error) {
	// Configurar opciones de RocksDB
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockCache(gorocksdb.NewLRUCache(3 << 30))
	bbto.SetFilterPolicy(gorocksdb.NewBloomFilter(10))

	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)
	opts.SetCompression(gorocksdb.SnappyCompression)
	opts.SetWriteBufferSize(64 * 1024 * 1024) // 64MB
	opts.SetMaxWriteBufferNumber(3)
	opts.SetTargetFileSizeBase(64 * 1024 * 1024) // 64MB

	// Abrir la base de datos
	db, err := gorocksdb.OpenDb(opts, dbPath)
	if err != nil {
		return nil, err
	}

	return &SensorDataCompressor{
		db:           db,
		readOptions:  gorocksdb.NewDefaultReadOptions(),
		writeOptions: gorocksdb.NewDefaultWriteOptions(),
		prevValues:   make(map[string]float64),
	}, nil
}

// Close libera los recursos
func (c *SensorDataCompressor) Close() {
	c.readOptions.Destroy()
	c.writeOptions.Destroy()
	c.db.Close()
}

// EncodeGorilla implementa la compresión Gorilla para series temporales
func (c *SensorDataCompressor) EncodeGorilla(timestamps []int64, values []float64) ([]byte, error) {
	if len(timestamps) == 0 || len(values) == 0 {
		return nil, fmt.Errorf("arrays vacíos")
	}

	buf := new(bytes.Buffer)

	// Primer timestamp completo (8 bytes)
	err := binary.Write(buf, binary.BigEndian, timestamps[0])
	if err != nil {
		return nil, err
	}

	// Delta de timestamps
	prevTs := timestamps[0]
	for i := 1; i < len(timestamps); i++ {
		delta := timestamps[i] - prevTs
		// Codificación variable
		for delta >= 128 {
			err = buf.WriteByte(byte((delta & 0x7f) | 0x80))
			if err != nil {
				return nil, err
			}
			delta >>= 7
		}
		err = buf.WriteByte(byte(delta & 0x7f))
		if err != nil {
			return nil, err
		}
		prevTs = timestamps[i]
	}

	// Primer valor completo (8 bytes)
	err = binary.Write(buf, binary.BigEndian, values[0])
	if err != nil {
		return nil, err
	}

	// XOR de valores para codificación Gorilla
	prevVal := values[0]
	for i := 1; i < len(values); i++ {
		// Convertir a representación binaria para obtener XOR
		prevBits := math.Float64bits(prevVal)
		currBits := math.Float64bits(values[i])

		// XOR para encontrar bits diferentes
		xor := prevBits ^ currBits

		if xor == 0 {
			// Sin cambio, guardar un bit '0'
			err = buf.WriteByte(0)
			if err != nil {
				return nil, err
			}
		} else {
			// Encontrar bits significativos usando el paquete bits
			leadingZeros := uint8(bits.LeadingZeros64(xor))
			trailingZeros := uint8(bits.TrailingZeros64(xor))

			// Guardar '1' seguido por lead+trail+valores significativos
			err = buf.WriteByte(1)
			if err != nil {
				return nil, err
			}

			err = buf.WriteByte(leadingZeros)
			if err != nil {
				return nil, err
			}

			err = buf.WriteByte(trailingZeros)
			if err != nil {
				return nil, err
			}

			// Guardar bits significativos
			significantBits := xor >> trailingZeros
			bytesNeeded := (64 - int(leadingZeros) - int(trailingZeros) + 7) / 8
			significantBytes := make([]byte, bytesNeeded)
			
			for j := bytesNeeded - 1; j >= 0; j-- {
				significantBytes[j] = byte(significantBits & 0xFF)
				significantBits >>= 8
			}
			
			_, err = buf.Write(significantBytes)
			if err != nil {
				return nil, err
			}
		}

		prevVal = values[i]
	}

	return buf.Bytes(), nil
}

// DecodeGorilla decodifica datos comprimidos con Gorilla
func (c *SensorDataCompressor) DecodeGorilla(data []byte) ([]int64, []float64, error) {
	if len(data) < 16 { // Al menos necesitamos el primer timestamp y valor
		return nil, nil, fmt.Errorf("datos insuficientes")
	}

	reader := bytes.NewReader(data)
	
	// Leer primer timestamp (8 bytes)
	var firstTs int64
	err := binary.Read(reader, binary.BigEndian, &firstTs)
	if err != nil {
		return nil, nil, err
	}
	
	// Leer primer valor (8 bytes)
	var firstVal float64
	err = binary.Read(reader, binary.BigEndian, &firstVal)
	if err != nil {
		return nil, nil, err
	}
	
	// Inicializar arrays de resultado
	timestamps := []int64{firstTs}
	values := []float64{firstVal}
	
	// La implementación completa de la decodificación requeriría más código
	// para procesar los deltas de timestamp y valores XOR
	// Esta es una versión simplificada
	
	return timestamps, values, nil
}

// ApplyDeltaEncoding aplica delta encoding a una lista de valores
func (c *SensorDataCompressor) ApplyDeltaEncoding(values []float64) []float64 {
	if len(values) == 0 {
		return []float64{}
	}

	result := make([]float64, len(values))
	result[0] = values[0] // El primer valor se mantiene igual

	for i := 1; i < len(values); i++ {
		result[i] = values[i] - values[i-1]
	}

	return result
}

// ApplyDeltaDecode recupera valores originales desde delta encoding
func (c *SensorDataCompressor) ApplyDeltaDecode(deltas []float64) []float64 {
	if len(deltas) == 0 {
		return []float64{}
	}

	result := make([]float64, len(deltas))
	result[0] = deltas[0]

	for i := 1; i < len(deltas); i++ {
		result[i] = result[i-1] + deltas[i]
	}

	return result
}

// ApplyDeltaDeltaEncoding aplica delta-delta encoding (segunda derivada)
func (c *SensorDataCompressor) ApplyDeltaDeltaEncoding(values []float64) []float64 {
	if len(values) < 2 {
		return values
	}

	// Primero delta encoding
	deltas := c.ApplyDeltaEncoding(values)

	// Luego delta encoding en los deltas
	result := make([]float64, len(deltas))
	result[0] = deltas[0] // El primer delta se mantiene

	for i := 1; i < len(deltas); i++ {
		result[i] = deltas[i] - deltas[i-1]
	}

	return result
}

// ApplyDeltaDeltaDecode recupera valores originales desde delta-delta encoding
func (c *SensorDataCompressor) ApplyDeltaDeltaDecode(deltadeltas []float64) []float64 {
	if len(deltadeltas) < 2 {
		return deltadeltas
	}

	// Primero recuperar deltas
	deltas := make([]float64, len(deltadeltas))
	deltas[0] = deltadeltas[0]

	for i := 1; i < len(deltadeltas); i++ {
		deltas[i] = deltadeltas[i] + deltas[i-1]
	}

	// Luego recuperar valores originales
	return c.ApplyDeltaDecode(deltas)
}

// ApplyRLEEncoding aplica Run-Length Encoding a valores
func (c *SensorDataCompressor) ApplyRLEEncoding(values []float64) []RLEEntry {
	if len(values) == 0 {
		return []RLEEntry{}
	}

	var result []RLEEntry
	currentVal := values[0]
	count := 1

	for i := 1; i < len(values); i++ {
		if values[i] == currentVal {
			count++
		} else {
			result = append(result, RLEEntry{Value: currentVal, Count: count})
			currentVal = values[i]
			count = 1
		}
	}

	// Añadir el último grupo
	result = append(result, RLEEntry{Value: currentVal, Count: count})
	return result
}

// ApplyRLEDecode recupera valores originales desde RLE
func (c *SensorDataCompressor) ApplyRLEDecode(encoded []RLEEntry) []float64 {
	var result []float64

	for _, entry := range encoded {
		for i := 0; i < entry.Count; i++ {
			result = append(result, entry.Value)
		}
	}

	return result
}

// StoreSensorReadings almacena lecturas de sensores con compresión
func (c *SensorDataCompressor) StoreSensorReadings(sensorID string, timestamps []int64, values []float64) error {
	if len(timestamps) != len(values) {
		return fmt.Errorf("el número de timestamps y valores debe coincidir")
	}

	// Verificar si tenemos suficientes datos para compresión efectiva
	if len(values) < 5 {
		// Almacenar sin compresión especial si hay pocos puntos
		batch := gorocksdb.NewWriteBatch()
		defer batch.Destroy()

		for i, ts := range timestamps {
			key := fmt.Sprintf("sensor:%s:readings:%d", sensorID, ts)
			value := strconv.FormatFloat(values[i], 'f', -1, 64)
			batch.Put([]byte(key), []byte(value))
		}

		return c.db.Write(c.writeOptions, batch)
	}

	// Determinar el tipo de compresión basado en las características de los datos
	var block CompressedBlock
	block.StartTime = timestamps[0]
	block.EndTime = timestamps[len(timestamps)-1]
	block.Count = len(values)

	// Revisar si los datos parecen discretos (posiblemente enteros)
	discreteData := true
	for _, v := range values {
		if v != math.Floor(v) {
			discreteData = false
			break
		}
	}

	// Para datos con tendencia (como temperatura), delta-delta es mejor
	ddValues := c.ApplyDeltaDeltaEncoding(values)

	if discreteData {
		// Usar RLE para valores discretos
		block.Compression = CompressRLE
		block.RLEData = c.ApplyRLEEncoding(ddValues)
	} else {
		// Usar Gorilla para valores continuos
		var err error
		block.Compression = CompressGorilla
		block.Data, err = c.EncodeGorilla(timestamps, values)
		if err != nil {
			// Si falla, usar delta-delta encoding
			block.Compression = CompressDeltaDelta
			ddBytes, err := json.Marshal(ddValues)
			if err != nil {
				return err
			}
			block.Data = ddBytes
		}
	}

	// Convertir el bloque a JSON
	blockData, err := json.Marshal(block)
	if err != nil {
		return err
	}

	// Almacenar el bloque comprimido
	batchKey := fmt.Sprintf("sensor:%s:compressed:%d:%d", sensorID, timestamps[0], timestamps[len(timestamps)-1])
	err = c.db.Put(c.writeOptions, []byte(batchKey), blockData)
	if err != nil {
		return err
	}

	// Añadir índices para buscar este bloque más rápido
	batch := gorocksdb.NewWriteBatch()
	defer batch.Destroy()

	indexStartKey := fmt.Sprintf("sensor:%s:index:%d", sensorID, timestamps[0])
	indexEndKey := fmt.Sprintf("sensor:%s:index:%d", sensorID, timestamps[len(timestamps)-1])
	batch.Put([]byte(indexStartKey), []byte(batchKey))
	batch.Put([]byte(indexEndKey), []byte(batchKey))

	return c.db.Write(c.writeOptions, batch)
}

// GetSensorReadings recupera lecturas de sensores en un rango de tiempo
func (c *SensorDataCompressor) GetSensorReadings(sensorID string, startTime, endTime int64) (map[int64]float64, error) {
	results := make(map[int64]float64)

	// Buscar bloques comprimidos
	it := c.db.NewIterator(c.readOptions)
	defer it.Close()

	searchKey := fmt.Sprintf("sensor:%s:compressed:", sensorID)
	it.Seek([]byte(searchKey))

	for it.Valid() {
		key := string(it.Key().Data())
		if !bytes.HasPrefix(it.Key().Data(), []byte(searchKey)) {
			break
		}

		// Extraer rango de tiempo del bloque
		var blockStart, blockEnd int64
		fmt.Sscanf(key, fmt.Sprintf("sensor:%s:compressed:%%d:%%d", sensorID), &blockStart, &blockEnd)

		if blockStart > endTime {
			break
		}

		if blockEnd >= startTime {
			// Este bloque contiene datos en el rango solicitado
			value := it.Value()
			var block CompressedBlock
			err := json.Unmarshal(value.Data(), &block)
			if err != nil {
				it.Next()
				continue
			}

			// Decodificar según el tipo de compresión
			var timestamps []int64
			var values []float64

			switch block.Compression {
			case CompressGorilla:
				timestamps, values, err = c.DecodeGorilla(block.Data)
				if err != nil {
					it.Next()
					continue
				}

			case CompressDeltaDelta:
				var ddValues []float64
				err := json.Unmarshal(block.Data, &ddValues)
				if err != nil {
					it.Next()
					continue
				}
				values = c.ApplyDeltaDeltaDecode(ddValues)

				// Reconstruir timestamps (asumiendo intervalos regulares)
				interval := float64(block.EndTime-block.StartTime) / float64(len(values)-1)
				timestamps = make([]int64, len(values))
				for i := range values {
					timestamps[i] = block.StartTime + int64(float64(i)*interval)
				}

			case CompressRLE:
				ddValues := c.ApplyRLEDecode(block.RLEData)
				values = c.ApplyDeltaDeltaDecode(ddValues)

				// Reconstruir timestamps
				interval := float64(block.EndTime-block.StartTime) / float64(len(values)-1)
				timestamps = make([]int64, len(values))
				for i := range values {
					timestamps[i] = block.StartTime + int64(float64(i)*interval)
				}
			}

			// Añadir al resultado los puntos dentro del rango
			for i, ts := range timestamps {
				if startTime <= ts && ts <= endTime {
					results[ts] = values[i]
				}
			}
		}

		it.Next()
	}

	// Buscar también lecturas individuales
	searchKey = fmt.Sprintf("sensor:%s:readings:%d", sensorID, startTime)
	it.Seek([]byte(searchKey))

	prefix := fmt.Sprintf("sensor:%s:readings:", sensorID)
	for it.Valid() {
		key := string(it.Key().Data())
		if !bytes.HasPrefix(it.Key().Data(), []byte(prefix)) {
			break
		}

		// Extraer timestamp
		var ts int64
		fmt.Sscanf(key, fmt.Sprintf("sensor:%s:readings:%%d", sensorID), &ts)

		if ts > endTime {
			break
		}

		if startTime <= ts && ts <= endTime {
			val, err := strconv.ParseFloat(string(it.Value().Data()), 64)
			if err == nil {
				results[ts] = val
			}
		}

		it.Next()
	}

	return results, nil
}

// Ejemplo completo de uso con datos de sensores
func main() {
	// Crear instancia del compresor
	compressor, err := NewSensorDataCompressor("sensor_data.db")
	if err != nil {
		log.Fatalf("Error al abrir la base de datos: %v", err)
	}
	defer compressor.Close()

	// Simulación de datos de sensores
	sensorID := "temp_sensor_001"
	now := time.Now().Unix()

	// Generar datos de ejemplo para temperatura
	var timestamps []int64
	var values []float64

	for i := 0; i < 100; i++ {
		timestamps = append(timestamps, now+int64(i*60)) // Lecturas cada minuto
		baseTemp := 22.5
		value := baseTemp + 0.1*float64(i) + 0.2*math.Sin(float64(i)/10)
		values = append(values, value)
	}

	// Almacenar usando compresión
	err = compressor.StoreSensorReadings(sensorID, timestamps, values)
	if err != nil {
		log.Fatalf("Error al almacenar lecturas: %v", err)
	}

	// Recuperar datos
	start := now + 10*60
	end := now + 50*60
	readings, err := compressor.GetSensorReadings(sensorID, start, end)
	if err != nil {
		log.Fatalf("Error al recuperar lecturas: %v", err)
	}

	fmt.Printf("Recuperados %d puntos de datos entre %d y %d\n", len(readings), start, end)

	// Mostrar algunos valores
	count := 0
	for ts, val := range readings {
		fmt.Printf("Tiempo: %s, Valor: %.2f\n", time.Unix(ts, 0).Format(time.RFC3339), val)
		count++
		if count >= 5 {
			break
		}
	}
}