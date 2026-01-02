package tipos

import (
	"path"
	"strings"
)

// Información de un nodo registrado
type Nodo struct {
	NodoID      string           // Identificador del nodo
	DireccionIP string           // Dirección IP del nodo
	PuertoHTTP  string           // Puerto HTTP del nodo
	Series      map[string]Serie // Lista de series gestionadas por el nodo
}

// Serie representa una serie de datos de tiempo
type Serie struct {
	SerieId              int                  // ID de la serie en la base de datos
	Path                 string               // Path jerárquico: "dispositivo_001/temperatura"
	Tags                 map[string]string    // Tags: {"unidad": "Celsius", "tipo": "DHT22"}
	TipoDatos            TipoDatos            // Tipo de datos almacenados
	CompresionBloque     TipoCompresionBloque // Compresión nivel bloque
	CompresionBytes      TipoCompresion       // Compresión nivel valores (algoritmos específicos por tipo)
	TamañoBloque         int                  // Tamaño del bloque
	TiempoAlmacenamiento int64                // Tiempo máximo de almacenamiento en nanosegundos (0 = sin límite)
}

// MatchPath verifica si un path coincide con un patrón glob.
// Soporta wildcard '*' que matchea cualquier secuencia de caracteres (sin /).
// Ejemplos:
//   - MatchPath("sensor_01/temp", "*/temp") -> true
//   - MatchPath("sensor_01/temp", "sensor_01/*") -> true
//   - MatchPath("sensor_01/temp", "*") -> true (caso especial)
//   - MatchPath("dispositivo1/temp", "dispositivo*/temp") -> true
func MatchPath(pathStr, patron string) bool {
	if patron == "*" {
		return true
	}
	matched, _ := path.Match(patron, pathStr)
	return matched
}

// EsPatronWildcard determina si un path contiene wildcards
func EsPatronWildcard(path string) bool {
	return strings.Contains(path, "*")
}
