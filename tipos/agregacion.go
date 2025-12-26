package tipos

// TipoAgregacion define los tipos de agregación soportados para consultas
type TipoAgregacion string

const (
	AgregacionPromedio TipoAgregacion = "promedio"
	AgregacionMaximo   TipoAgregacion = "maximo"
	AgregacionMinimo   TipoAgregacion = "minimo"
	AgregacionSuma     TipoAgregacion = "suma"
	AgregacionCount    TipoAgregacion = "count"
)

// ResultadoAgregacionTemporal representa el resultado de una agregación temporal en formato matricial.
// Cada serie temporal es una columna, los buckets de tiempo son las filas.
// Valores faltantes se representan como math.NaN().
type ResultadoAgregacionTemporal struct {
	Series  []string    // Columnas: nombres de series ordenados alfabéticamente
	Tiempos []int64     // Filas: inicio de cada bucket (Unix nanosegundos)
	Valores [][]float64 // Matriz [bucket][serie], math.NaN() = sin datos en ese bucket
}
