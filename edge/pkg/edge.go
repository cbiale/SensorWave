// Package edgesensorwave proporciona una base de datos embebida optimizada para series temporales IoT
// usando Pebble como motor de almacenamiento backend.
//
// EdgeSensorWave está diseñado específicamente para aplicaciones edge computing que requieren:
// - Alto rendimiento (100,000+ escrituras/segundo)
// - Footprint de memoria mínimo (<50MB)
// - API específica para sensores IoT
// - Persistencia confiable con Pebble
//
// Ejemplo de uso básico:
//
//	db, err := edgesensorwave.Abrir("sensores.esw", edgesensorwave.OpcionesDefecto())
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer db.Cerrar()
//
//	// Insertar datos de sensor
//	err = db.InsertarSensor("temperatura.salon", 23.5, time.Now())
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Consultar rango temporal
//	iter, err := db.ConsultarRango("temperatura.*", hace1Hora, ahora)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer iter.Cerrar()
//
//	for iter.Siguiente() {
//		clave := iter.Clave()
//		valor := iter.Valor()
//		fmt.Printf("%s: %.2f (calidad: %s)\n", 
//			clave.IDSensor, valor.Valor, valor.Calidad)
//	}
package edgesensorwave

import (
	"fmt"
	"time"

	"edgesensorwave/pkg/motor"
	"edgesensorwave/pkg/pebble_backend"
)

// DB representa una base de datos EdgeSensorWave
type DB struct {
	backend   *pebble_backend.DB
	consultas *pebble_backend.ConsultasAvanzadas
}

// Abrir abre o crea una base de datos EdgeSensorWave en la ruta especificada
func Abrir(ruta string, opciones *motor.Opciones) (*DB, error) {
	backend, err := pebble_backend.Abrir(ruta, opciones)
	if err != nil {
		return nil, fmt.Errorf("error abriendo backend: %w", err)
	}
	
	db := &DB{
		backend:   backend,
		consultas: backend.NuevasConsultasAvanzadas(),
	}
	
	return db, nil
}

// InsertarSensor inserta un valor de sensor con timestamp actual
func (db *DB) InsertarSensor(idSensor string, valor float64, timestamp time.Time) error {
	return db.backend.InsertarSensor(idSensor, valor, timestamp)
}

// InsertarSensorConCalidad inserta un valor de sensor con calidad y metadatos específicos
func (db *DB) InsertarSensorConCalidad(idSensor string, valor float64, calidad motor.CalidadDato, timestamp time.Time, metadatos map[string]string) error {
	return db.backend.InsertarSensorConCalidad(idSensor, valor, calidad, timestamp, metadatos)
}

// NuevoLote crea un nuevo lote para operaciones masivas
func (db *DB) NuevoLote() motor.Lote {
	return db.backend.NuevoLote()
}

// ConfirmarLote confirma y aplica todas las operaciones del lote a la base de datos
func (db *DB) ConfirmarLote(lote motor.Lote) error {
	return db.backend.ConfirmarLote(lote)
}

// ConsultarRango retorna un iterador para consultar datos en un rango temporal
// El patrón puede incluir wildcards, ej: "temperatura.*" para todos los sensores de temperatura
func (db *DB) ConsultarRango(patron string, inicio, fin time.Time) (motor.Iterador, error) {
	return db.backend.ConsultarRango(patron, inicio, fin)
}

// ConsultarRangoConLimite retorna un iterador con límite máximo de resultados
func (db *DB) ConsultarRangoConLimite(patron string, inicio, fin time.Time, limite int) (motor.Iterador, error) {
	return db.backend.ConsultarRangoConLimite(patron, inicio, fin, limite)
}

// BuscarSensor busca un valor específico de sensor en un timestamp exacto
func (db *DB) BuscarSensor(idSensor string, timestamp time.Time) (*motor.ValorSensor, error) {
	return db.backend.BuscarSensor(idSensor, timestamp)
}

// Estadisticas retorna estadísticas generales de la base de datos
func (db *DB) Estadisticas() (*motor.Estadisticas, error) {
	return db.backend.Estadisticas()
}

// Info retorna información detallada de la base de datos
func (db *DB) Info() (map[string]interface{}, error) {
	return db.backend.Info()
}

// Sincronizar fuerza la sincronización de datos al disco
func (db *DB) Sincronizar() error {
	return db.backend.Sincronizar()
}

// Compactar fuerza la compactación de la base de datos para optimizar espacio
func (db *DB) Compactar() error {
	return db.backend.Compactar()
}

// Cerrar cierra la base de datos y libera todos los recursos
func (db *DB) Cerrar() error {
	return db.backend.Cerrar()
}

// ===== API DE CONSULTAS AVANZADAS =====

// BuscarUltimo busca el valor más reciente de un sensor
func (db *DB) BuscarUltimo(idSensor string) (*motor.ClaveSensor, *motor.ValorSensor, error) {
	return db.consultas.BuscarUltimo(idSensor)
}

// BuscarPrimero busca el primer valor registrado de un sensor
func (db *DB) BuscarPrimero(idSensor string) (*motor.ClaveSensor, *motor.ValorSensor, error) {
	return db.consultas.BuscarPrimero(idSensor)
}

// CalcularEstadisticas calcula estadísticas completas para un sensor en un rango temporal
func (db *DB) CalcularEstadisticas(idSensor string, inicio, fin time.Time) (*pebble_backend.EstadisticasSensor, error) {
	return db.consultas.CalcularEstadisticas(idSensor, inicio, fin)
}

// ListarSensores retorna una lista de todos los sensores que coincidan con un patrón
func (db *DB) ListarSensores(patron string) ([]string, error) {
	return db.consultas.ListarSensores(patron)
}

// AgregarPorIntervalo agrupa datos por intervalos de tiempo y calcula estadísticas agregadas
func (db *DB) AgregarPorIntervalo(idSensor string, inicio, fin time.Time, intervalo time.Duration) ([]pebble_backend.DatosAgregados, error) {
	return db.consultas.AgregarPorIntervalo(idSensor, inicio, fin, intervalo)
}

// BuscarAnomalias detecta valores que se desvían significativamente del promedio
func (db *DB) BuscarAnomalias(idSensor string, inicio, fin time.Time, umbralDesviaciones float64) ([]motor.ClaveSensor, error) {
	return db.consultas.BuscarAnomalias(idSensor, inicio, fin, umbralDesviaciones)
}

// BusquedaMultiple busca en múltiples sensores con scoring de relevancia
func (db *DB) BusquedaMultiple(patrones []string, inicio, fin time.Time, limite int) ([]pebble_backend.ResultadoBusqueda, error) {
	return db.consultas.BusquedaMultiple(patrones, inicio, fin, limite)
}

// ConsultarEnVentana consulta datos en una ventana temporal centrada en un momento específico
func (db *DB) ConsultarEnVentana(idSensor string, centro time.Time, ventana time.Duration) (motor.Iterador, error) {
	return db.consultas.ConsultarEnVentana(idSensor, centro, ventana)
}

// ContarRegistros cuenta el número de registros que coinciden con el patrón en el rango
func (db *DB) ContarRegistros(patron string, inicio, fin time.Time) (int64, error) {
	return db.consultas.ContarRegistros(patron, inicio, fin)
}

// TieneDatos verifica si un sensor tiene datos en un rango temporal
func (db *DB) TieneDatos(idSensor string, inicio, fin time.Time) (bool, error) {
	return db.consultas.TieneDatos(idSensor, inicio, fin)
}

// ===== FUNCIONES DE CONVENIENCIA =====

// OpcionesDefecto retorna las opciones de configuración por defecto
func OpcionesDefecto() *motor.Opciones {
	return motor.OpcionesDefecto()
}

// OpcionesRendimiento retorna opciones optimizadas para máximo rendimiento
func OpcionesRendimiento() *motor.Opciones {
	return motor.OpcionesRendimiento()
}

// OpcionesMemoriaMinima retorna opciones optimizadas para mínimo uso de memoria
func OpcionesMemoriaMinima() *motor.Opciones {
	return motor.OpcionesMemoriaMinima()
}

// ===== FUNCIONES DE UTILIDAD =====

// ValidarRuta verifica que una ruta sea válida para una base de datos
func ValidarRuta(ruta string) error {
	if ruta == "" {
		return fmt.Errorf("ruta vacía")
	}
	
	if len(ruta) > 255 {
		return fmt.Errorf("ruta demasiado larga: %d caracteres", len(ruta))
	}
	
	return nil
}

// InsertarLote es una función de conveniencia para insertar múltiples valores en una sola operación
func (db *DB) InsertarLote(datos []DatoSensor) error {
	if len(datos) == 0 {
		return nil
	}
	
	lote := db.NuevoLote()
	
	for _, dato := range datos {
		if dato.Metadatos != nil {
			err := lote.AgregarConCalidad(dato.IDSensor, dato.Valor, dato.Calidad, dato.Timestamp, dato.Metadatos)
			if err != nil {
				return fmt.Errorf("error agregando al lote: %w", err)
			}
		} else {
			err := lote.Agregar(dato.IDSensor, dato.Valor, dato.Timestamp)
			if err != nil {
				return fmt.Errorf("error agregando al lote: %w", err)
			}
		}
	}
	
	return db.ConfirmarLote(lote)
}

// DatoSensor representa un punto de datos para inserción en lote
type DatoSensor struct {
	IDSensor  string
	Valor     float64
	Calidad   motor.CalidadDato
	Timestamp time.Time
	Metadatos map[string]string
}

// ExportarRango exporta todos los datos de un rango a un slice para procesamiento externo
func (db *DB) ExportarRango(patron string, inicio, fin time.Time) ([]DatoSensorCompleto, error) {
	iter, err := db.ConsultarRango(patron, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	var resultados []DatoSensorCompleto
	
	for iter.Siguiente() {
		clave := iter.Clave()
		valor := iter.Valor()
		
		resultados = append(resultados, DatoSensorCompleto{
			IDSensor:  clave.IDSensor,
			Valor:     valor.Valor,
			Calidad:   valor.Calidad,
			Timestamp: clave.Timestamp,
			Metadatos: valor.Metadatos,
		})
	}
	
	return resultados, nil
}

// DatoSensorCompleto representa un punto de datos completo exportado
type DatoSensorCompleto struct {
	IDSensor  string
	Valor     float64
	Calidad   motor.CalidadDato
	Timestamp time.Time
	Metadatos map[string]string
}

// Version retorna la versión de EdgeSensorWave
func Version() string {
	return "1.0.0"
}

// VersionMotor retorna información sobre el motor de almacenamiento
func VersionMotor() string {
	return "Pebble-1.0"
}