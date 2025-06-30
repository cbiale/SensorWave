package motor

import (
	"time"
)

// CalidadDato representa la calidad de un dato de sensor
type CalidadDato int

const (
	CalidadBuena CalidadDato = iota
	CalidadSospechosa
	CalidadMala
	CalidadDesconocida
)

func (c CalidadDato) String() string {
	switch c {
	case CalidadBuena:
		return "Buena"
	case CalidadSospechosa:
		return "Sospechosa"
	case CalidadMala:
		return "Mala"
	case CalidadDesconocida:
		return "Desconocida"
	default:
		return "Desconocida"
	}
}

// ClaveSensor representa la clave única para un punto de datos de sensor
// Incluye el ID del sensor y el timestamp para ordenamiento temporal
type ClaveSensor struct {
	IDSensor  string
	Timestamp time.Time
}

// ValorSensor representa el valor almacenado para un sensor
// Incluye el valor numérico, calidad del dato y metadatos opcionales
type ValorSensor struct {
	Valor     float64
	Calidad   CalidadDato
	Metadatos map[string]string
}

// Iterador define la interfaz para iterar sobre resultados de consultas
type Iterador interface {
	Siguiente() bool
	Clave() *ClaveSensor
	Valor() *ValorSensor
	Cerrar() error
}

// Lote define la interfaz para operaciones de lote
type Lote interface {
	Agregar(idSensor string, valor float64, timestamp time.Time) error
	AgregarConCalidad(idSensor string, valor float64, calidad CalidadDato, timestamp time.Time, metadatos map[string]string) error
	Tamaño() int
	Limpiar()
}

// Estadisticas contiene información sobre el estado de la base de datos
type Estadisticas struct {
	NumSensores       int64
	NumRegistros      int64
	TamañoBytes       int64
	UltimaCompactacion time.Time
	VersionMotor      string
}