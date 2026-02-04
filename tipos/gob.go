package tipos

import (
	"bytes"
	"encoding/gob"
)

// ============================================================================
// STRUCTS DE CONSULTA COMPARTIDOS
// ============================================================================

// SolicitudConsultaRango representa una solicitud de consulta por rango de tiempo
type SolicitudConsultaRango struct {
	Serie        string
	TiempoInicio int64 // Unix nanosegundos
	TiempoFin    int64 // Unix nanosegundos
}

// SolicitudConsultaPunto representa una solicitud de último punto
// Los campos TiempoInicio y TiempoFin son opcionales:
//   - Si ambos son nil: retorna el último punto absoluto de cada serie
//   - Si se especifican: retorna el último punto dentro del rango temporal
type SolicitudConsultaPunto struct {
	Serie        string
	TiempoInicio *int64 // nil = sin límite inferior (Unix nanosegundos)
	TiempoFin    *int64 // nil = sin límite superior (Unix nanosegundos)
}

// ResultadoConsultaPunto representa el último punto de múltiples series en formato columnar.
// Cada serie tiene su último punto (timestamp y valor).
// Series sin datos son excluidas del resultado.
type ResultadoConsultaPunto struct {
	Series             []string      // Nombres de series ordenados alfabéticamente
	Tiempos            []int64       // Timestamp del punto por serie (Unix nanosegundos)
	Valores            []interface{} // Valor del punto por serie
	NodosNoDisponibles []string      // IDs de nodos que no respondieron (solo en consultas globales)
}

// ResultadoConsultaRango representa el resultado de una consulta de rango en formato tabular.
// Cada serie temporal es una columna, los timestamps son las filas.
// Valores faltantes se representan como nil.
type ResultadoConsultaRango struct {
	Series             []string        // Columnas: nombres de series ordenados alfabéticamente
	Tiempos            []int64         // Filas: timestamps únicos ordenados ascendente (Unix nanosegundos)
	Valores            [][]interface{} // Matriz [fila][columna], nil = valor faltante
	NodosNoDisponibles []string        // IDs de nodos que no respondieron (solo en consultas globales)
}

// RespuestaConsultaRango respuesta con resultado tabular de consulta por rango
type RespuestaConsultaRango struct {
	Resultado ResultadoConsultaRango
	Error     string
}

// RespuestaConsultaPunto respuesta con resultado de consulta de último punto en formato columnar
type RespuestaConsultaPunto struct {
	Resultado ResultadoConsultaPunto
	Error     string
}

// SolicitudConsultaAgregacion representa una solicitud de agregación (soporta múltiples)
type SolicitudConsultaAgregacion struct {
	Serie        string
	TiempoInicio int64            // Unix nanosegundos
	TiempoFin    int64            // Unix nanosegundos
	Agregaciones []TipoAgregacion // Lista de agregaciones a calcular
}

// SolicitudConsultaAgregacionTemporal representa una solicitud de downsampling (soporta múltiples)
type SolicitudConsultaAgregacionTemporal struct {
	Serie        string
	TiempoInicio int64            // Unix nanosegundos
	TiempoFin    int64            // Unix nanosegundos
	Agregaciones []TipoAgregacion // Lista de agregaciones a calcular
	Intervalo    int64            // Duration en nanosegundos
}

// ResultadoAgregacion representa el resultado columnar de múltiples agregaciones.
// Soporta múltiples agregaciones en una sola consulta.
// Estructura de Valores: [agregacion][serie]
type ResultadoAgregacion struct {
	Series             []string         // Nombres de series ordenados alfabéticamente
	Agregaciones       []TipoAgregacion // Lista ordenada de agregaciones calculadas
	Valores            [][]float64      // Matriz [agregacion][serie]
	NodosNoDisponibles []string         // IDs de nodos que no respondieron (solo en consultas globales)
}

// RespuestaConsultaAgregacion respuesta con resultado de agregación columnar
type RespuestaConsultaAgregacion struct {
	Resultado ResultadoAgregacion
	Error     string
}

// RespuestaConsultaAgregacionTemporal respuesta con resultado de downsampling en formato matricial
type RespuestaConsultaAgregacionTemporal struct {
	Resultado ResultadoAgregacionTemporal
	Error     string
}

// ============================================================================
// FUNCIONES DE SERIALIZACIÓN GOB
// ============================================================================

// SerializarGob serializa un valor usando Gob
func SerializarGob(v interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DeserializarGob deserializa bytes usando Gob
func DeserializarGob(data []byte, v interface{}) error {
	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)
	return decoder.Decode(v)
}

// ============================================================================
// REGISTRO DE TIPOS GOB
// ============================================================================

func init() {
	// Tipos de consulta
	gob.Register(SolicitudConsultaRango{})
	gob.Register(SolicitudConsultaPunto{})
	gob.Register(ResultadoConsultaRango{})
	gob.Register(ResultadoConsultaPunto{})
	gob.Register(RespuestaConsultaRango{})
	gob.Register(RespuestaConsultaPunto{})

	// Tipos para matriz de valores (interface{} puede contener nil o valores)
	gob.Register([]interface{}{})

	// Tipos de consulta de agregación
	gob.Register(SolicitudConsultaAgregacion{})
	gob.Register(SolicitudConsultaAgregacionTemporal{})
	gob.Register(ResultadoAgregacion{})
	gob.Register(RespuestaConsultaAgregacion{})
	gob.Register(RespuestaConsultaAgregacionTemporal{})
	gob.Register(ResultadoAgregacionTemporal{})
	gob.Register([][]float64{})
	gob.Register([][][]float64{})
	gob.Register([]TipoAgregacion{})

	// Tipos de datos
	gob.Register(Medicion{})
	gob.Register([]Medicion{})
	gob.Register(Serie{})

	// Tipos de valores que pueden estar en Medicion.Valor
	gob.Register(int64(0))
	gob.Register(float64(0))
	gob.Register(bool(false))
	gob.Register("")
}
