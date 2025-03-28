package models

// Sensor representa un tipo de sensor con su ID, nombre y tipo de métrica.
type Sensor struct {
	Id   int          // Identificador único del tipo de sensor
	Name string       // Nombre del tipo de sensor
	MetricType string // Tipo de métrica
}

// Actuator representa un tipo de actuador con su ID, nombre y estados posibles.
type Actuator struct {
	Id     int      // Identificador único del tipo de actuador
	Name   string   // Nombre del tipo de actuador
	States []string // Lista de estados posibles
}

// Node representa un nodo del sistema con su ID, nombre y metadatos.
type Node struct {
	Id       int                // Identificador único del nodo
	Name     string             // Nombre del nodo
	Metadata map[string]string  // Metadatos del nodo
}
