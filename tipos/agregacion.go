package tipos

import "time"

// TipoAgregacion define los tipos de agregación soportados para consultas
type TipoAgregacion string

const (
	AgregacionPromedio TipoAgregacion = "promedio"
	AgregacionMaximo   TipoAgregacion = "maximo"
	AgregacionMinimo   TipoAgregacion = "minimo"
	AgregacionSuma     TipoAgregacion = "suma"
	AgregacionCount    TipoAgregacion = "count"
)

// ResultadoAgregacionTemporal representa un valor agregado para un bucket temporal
// Usado por ConsultarAgregacionTemporal para retornar resultados de downsampling
type ResultadoAgregacionTemporal struct {
	Tiempo time.Time // Inicio del bucket temporal
	Valor  float64   // Valor agregado para este bucket
}
