package tipos

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
	Tags                 map[string]string    // Tags: {"ubicacion": "sala1", "tipo": "DHT22"}
	TipoDatos            TipoDatos            // Tipo de datos almacenados
	CompresionBloque     TipoCompresionBloque // Compresión nivel bloque
	CompresionBytes      TipoCompresion       // Compresión nivel valores (algoritmos específicos por tipo)
	TamañoBloque         int                  // Tamaño del bloque
	TiempoAlmacenamiento int64                // Tiempo máximo de almacenamiento en nanosegundos (0 = sin límite)
}
