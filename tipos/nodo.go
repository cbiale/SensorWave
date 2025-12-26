package tipos

import "strings"

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

// MatchPath verifica si un path coincide con un patrón que soporta wildcard (*).
// El wildcard * solo funciona como segmento completo (ej: */temp, sensor_01/*).
// Ejemplos:
//   - MatchPath("sensor_01/temp", "*/temp") -> true
//   - MatchPath("sensor_01/temp", "sensor_01/*") -> true
//   - MatchPath("sensor_01/temp", "*") -> true
//   - MatchPath("sensor_01/temp", "sensor_02/temp") -> false
func MatchPath(path, patron string) bool {
	if patron == "*" {
		return true
	}
	if !strings.Contains(patron, "*") {
		return path == patron
	}
	partes := strings.Split(patron, "/")
	pathPartes := strings.Split(path, "/")
	if len(partes) != len(pathPartes) {
		return false
	}
	for i, parte := range partes {
		if parte == "*" {
			continue
		}
		if parte != pathPartes[i] {
			return false
		}
	}
	return true
}

// EsPatronWildcard determina si un path contiene wildcards
func EsPatronWildcard(path string) bool {
	return strings.Contains(path, "*")
}
