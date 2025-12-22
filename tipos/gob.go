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

// SolicitudConsultaPunto representa una solicitud de primer/último punto
type SolicitudConsultaPunto struct {
	Serie string
}

// RespuestaConsultaRango respuesta con múltiples mediciones
type RespuestaConsultaRango struct {
	Mediciones []Medicion
	Error      string
}

// RespuestaConsultaPunto respuesta con una medición
type RespuestaConsultaPunto struct {
	Medicion   Medicion
	Encontrado bool
	Error      string
}

// SolicitudConsultaAgregacion representa una solicitud de agregación simple
type SolicitudConsultaAgregacion struct {
	Serie        string
	TiempoInicio int64 // Unix nanosegundos
	TiempoFin    int64 // Unix nanosegundos
	Agregacion   TipoAgregacion
}

// SolicitudConsultaAgregacionTemporal representa una solicitud de downsampling
type SolicitudConsultaAgregacionTemporal struct {
	Serie        string
	TiempoInicio int64 // Unix nanosegundos
	TiempoFin    int64 // Unix nanosegundos
	Agregacion   TipoAgregacion
	Intervalo    int64 // Duration en nanosegundos
}

// RespuestaConsultaAgregacion respuesta con valor agregado
type RespuestaConsultaAgregacion struct {
	Valor float64
	Error string
}

// RespuestaConsultaAgregacionTemporal respuesta con resultados de downsampling
type RespuestaConsultaAgregacionTemporal struct {
	Resultados []ResultadoAgregacionTemporal
	Error      string
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
	gob.Register(RespuestaConsultaRango{})
	gob.Register(RespuestaConsultaPunto{})

	// Tipos de consulta de agregación
	gob.Register(SolicitudConsultaAgregacion{})
	gob.Register(SolicitudConsultaAgregacionTemporal{})
	gob.Register(RespuestaConsultaAgregacion{})
	gob.Register(RespuestaConsultaAgregacionTemporal{})
	gob.Register(ResultadoAgregacionTemporal{})
	gob.Register([]ResultadoAgregacionTemporal{})

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
