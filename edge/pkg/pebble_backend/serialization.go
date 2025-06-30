package pebble_backend

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"edgesensorwave/pkg/motor"
)

const (
	// Prefijos para diferentes tipos de claves
	PrefijoSensor     = byte(0x01)
	PrefijoIndice     = byte(0x02)
	PrefijoMetadatos  = byte(0x03)
	
	// Tamaños fijos para serialización optimizada
	TamañoTimestamp = 8 // int64 unix nano
	TamañoFloat64   = 8
	TamañoCalidad   = 1
)

// SerializarClave convierte ClaveSensor a bytes para usar como clave en Pebble
// Formato: [prefijo:1][timestamp:8][idSensor:variable]
// Esto garantiza ordenamiento temporal primero, luego por ID de sensor
func SerializarClave(clave *motor.ClaveSensor) []byte {
	idBytes := []byte(clave.IDSensor)
	resultado := make([]byte, 1+TamañoTimestamp+len(idBytes))
	
	// Prefijo para identificar tipo de clave
	resultado[0] = PrefijoSensor
	
	// Timestamp como int64 (unix nano) para ordenamiento temporal
	timestamp := clave.Timestamp.UnixNano()
	binary.BigEndian.PutUint64(resultado[1:9], uint64(timestamp))
	
	// ID del sensor al final
	copy(resultado[9:], idBytes)
	
	return resultado
}

// DeserializarClave convierte bytes de vuelta a ClaveSensor
func DeserializarClave(datos []byte) (*motor.ClaveSensor, error) {
	if len(datos) < 1+TamañoTimestamp {
		return nil, fmt.Errorf("datos de clave demasiado cortos: %d bytes", len(datos))
	}
	
	// Verificar prefijo
	if datos[0] != PrefijoSensor {
		return nil, fmt.Errorf("prefijo de clave inválido: %x", datos[0])
	}
	
	// Extraer timestamp
	timestampUint := binary.BigEndian.Uint64(datos[1:9])
	timestamp := time.Unix(0, int64(timestampUint))
	
	// Extraer ID del sensor
	idSensor := string(datos[9:])
	
	return &motor.ClaveSensor{
		IDSensor:  idSensor,
		Timestamp: timestamp,
	}, nil
}

// SerializarValor convierte ValorSensor a bytes para almacenar en Pebble
// Formato: [valor:8][calidad:1][metadatos_json:variable]
func SerializarValor(valor *motor.ValorSensor) ([]byte, error) {
	// Serializar metadatos como JSON
	var metadatosBytes []byte
	var err error
	if valor.Metadatos != nil && len(valor.Metadatos) > 0 {
		metadatosBytes, err = json.Marshal(valor.Metadatos)
		if err != nil {
			return nil, fmt.Errorf("error serializando metadatos: %w", err)
		}
	}
	
	// Calcular tamaño total
	tamaño := TamañoFloat64 + TamañoCalidad + len(metadatosBytes)
	resultado := make([]byte, tamaño)
	
	// Serializar valor como float64
	bits := math.Float64bits(valor.Valor)
	binary.BigEndian.PutUint64(resultado[0:8], bits)
	
	// Serializar calidad como byte
	resultado[8] = byte(valor.Calidad)
	
	// Agregar metadatos si existen
	if len(metadatosBytes) > 0 {
		copy(resultado[9:], metadatosBytes)
	}
	
	return resultado, nil
}

// DeserializarValor convierte bytes de vuelta a ValorSensor
func DeserializarValor(datos []byte) (*motor.ValorSensor, error) {
	if len(datos) < TamañoFloat64+TamañoCalidad {
		return nil, fmt.Errorf("datos de valor demasiado cortos: %d bytes", len(datos))
	}
	
	// Deserializar valor float64
	bits := binary.BigEndian.Uint64(datos[0:8])
	valor := math.Float64frombits(bits)
	
	// Deserializar calidad
	calidad := motor.CalidadDato(datos[8])
	
	// Deserializar metadatos si existen
	var metadatos map[string]string
	if len(datos) > TamañoFloat64+TamañoCalidad {
		metadatosBytes := datos[9:]
		if len(metadatosBytes) > 0 {
			if err := json.Unmarshal(metadatosBytes, &metadatos); err != nil {
				return nil, fmt.Errorf("error deserializando metadatos: %w", err)
			}
		}
	}
	
	return &motor.ValorSensor{
		Valor:     valor,
		Calidad:   calidad,
		Metadatos: metadatos,
	}, nil
}

// CrearClaveRango crea claves para búsquedas de rango temporal
// Útil para ConsultarRango con timestamps de inicio y fin
func CrearClaveRango(idSensor string, timestamp time.Time) []byte {
	clave := &motor.ClaveSensor{
		IDSensor:  idSensor,
		Timestamp: timestamp,
	}
	return SerializarClave(clave)
}

// CrearPrefijoBusqueda crea un prefijo para búsquedas por patrón
// Ejemplo: "temperatura.*" -> prefijo para todos los sensores que empiecen con "temperatura."
func CrearPrefijoBusqueda(patron string) []byte {
	// Para patrones con wildcard (*), usar solo la parte fija
	prefijo := patron
	if idx := len(patron); idx > 0 && patron[idx-1] == '*' {
		prefijo = patron[:idx-1]
	}
	
	// Crear clave con timestamp mínimo para comenzar búsqueda
	clave := &motor.ClaveSensor{
		IDSensor:  prefijo,
		Timestamp: time.Time{}, // Tiempo mínimo
	}
	
	return SerializarClave(clave)
}

// ExtraerIDSensorDeClave extrae solo el ID del sensor de una clave serializada
// Útil para filtrado rápido sin deserializar completamente
func ExtraerIDSensorDeClave(clave []byte) (string, error) {
	if len(clave) < 1+TamañoTimestamp {
		return "", fmt.Errorf("clave demasiado corta")
	}
	
	if clave[0] != PrefijoSensor {
		return "", fmt.Errorf("prefijo de clave inválido")
	}
	
	return string(clave[9:]), nil
}

// CompararClaves compara dos claves serializadas para ordenamiento
// Retorna: -1 si a < b, 0 si a == b, 1 si a > b
func CompararClaves(a, b []byte) int {
	// Pebble maneja la comparación de bytes automáticamente
	// pero esta función puede ser útil para tests o debugging
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	
	return 0
}