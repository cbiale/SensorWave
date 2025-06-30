package pebble_backend

import (
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"edgesensorwave/pkg/motor"
)

// DB implementa la base de datos EdgeSensorWave usando Pebble como backend
type DB struct {
	pb       *pebble.DB
	opciones *motor.Opciones
	mutex    sync.RWMutex
	cerrada  bool
	
	// Estadísticas
	contadorSensores  map[string]bool
	contadorRegistros int64
	ultimaCompactacion time.Time
}

// Abrir crea una nueva instancia de base de datos EdgeSensorWave
func Abrir(ruta string, opciones *motor.Opciones) (*DB, error) {
	if opciones == nil {
		opciones = motor.OpcionesDefecto()
	}
	
	// Validar opciones
	if err := opciones.Validar(); err != nil {
		return nil, fmt.Errorf("opciones inválidas: %w", err)
	}
	
	// Configurar Pebble para IoT edge
	cache := pebble.NewCache(opciones.CacheBytes)
	defer cache.Unref()
	
	pebbleOpts := &pebble.Options{
		// Configuración de memoria optimizada para edge
		Cache:                 cache,
		MemTableSize:          uint64(opciones.MemTableBytes),
		MaxOpenFiles:          opciones.MaxOpenFiles,	
		// Configuración de compactación no-blocking  
		// No deshabilitar WAL ya que es necesario para la integridad
		MaxConcurrentCompactions: func() int {
			if opciones.CompactacionParalela {
				return 2
			}
			return 1
		},
		
		// Configuración de L0
		L0CompactionThreshold: opciones.CompactacionNivelMinimo,
		L0StopWritesThreshold: opciones.CompactacionNivelMinimo * 4,
	}
	
	// Abrir base de datos Pebble
	pb, err := pebble.Open(ruta, pebbleOpts)
	if err != nil {
		return nil, fmt.Errorf("error abriendo base de datos Pebble: %w", err)
	}
	
	db := &DB{
		pb:                pb,
		opciones:          opciones,
		contadorSensores:  make(map[string]bool),
		ultimaCompactacion: time.Now(),
	}
	
	// Cargar estadísticas existentes
	if err := db.cargarEstadisticas(); err != nil {
		// Log warning pero no fallar
		_ = err
	}
	
	return db, nil
}

// InsertarSensor inserta un valor de sensor con timestamp
func (db *DB) InsertarSensor(idSensor string, valor float64, timestamp time.Time) error {
	return db.InsertarSensorConCalidad(idSensor, valor, motor.CalidadBuena, timestamp, nil)
}

// InsertarSensorConCalidad inserta un valor de sensor con calidad y metadatos
func (db *DB) InsertarSensorConCalidad(idSensor string, valor float64, calidad motor.CalidadDato, timestamp time.Time, metadatos map[string]string) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	if db.cerrada {
		return fmt.Errorf("base de datos cerrada")
	}
	
	// Validar entrada si está habilitado
	if db.opciones.ValidarIDs {
		if err := validarIDSensor(idSensor); err != nil {
			return fmt.Errorf("ID de sensor inválido: %w", err)
		}
	}
	
	// Crear clave y valor
	clave := &motor.ClaveSensor{
		IDSensor:  idSensor,
		Timestamp: timestamp,
	}
	
	valorSensor := &motor.ValorSensor{
		Valor:     valor,
		Calidad:   calidad,
		Metadatos: metadatos,
	}
	
	// Serializar
	claveBytes := SerializarClave(clave)
	valorBytes, err := SerializarValor(valorSensor)
	if err != nil {
		return fmt.Errorf("error serializando valor: %w", err)
	}
	
	// Escribir a Pebble
	if err := db.pb.Set(claveBytes, valorBytes, pebble.Sync); err != nil {
		return fmt.Errorf("error escribiendo a Pebble: %w", err)
	}
	
	// Actualizar estadísticas
	db.contadorSensores[idSensor] = true
	db.contadorRegistros++
	
	return nil
}

// BuscarSensor busca un valor específico de sensor en un timestamp
func (db *DB) BuscarSensor(idSensor string, timestamp time.Time) (*motor.ValorSensor, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	if db.cerrada {
		return nil, fmt.Errorf("base de datos cerrada")
	}
	
	// Crear clave de búsqueda
	clave := &motor.ClaveSensor{
		IDSensor:  idSensor,
		Timestamp: timestamp,
	}
	
	claveBytes := SerializarClave(clave)
	
	// Buscar en Pebble
	valorBytes, closer, err := db.pb.Get(claveBytes)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando en Pebble: %w", err)
	}
	defer closer.Close()
	
	// Deserializar valor
	valor, err := DeserializarValor(valorBytes)
	if err != nil {
		return nil, fmt.Errorf("error deserializando valor: %w", err)
	}
	
	return valor, nil
}

// Estadisticas retorna estadísticas de la base de datos
func (db *DB) Estadisticas() (*motor.Estadisticas, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	if db.cerrada {
		return nil, fmt.Errorf("base de datos cerrada")
	}
	
	// Obtener métricas de Pebble
	metrics := db.pb.Metrics()
	
	stats := &motor.Estadisticas{
		NumSensores:        int64(len(db.contadorSensores)),
		NumRegistros:       db.contadorRegistros,
		TamañoBytes:        int64(metrics.DiskSpaceUsage()),
		UltimaCompactacion: db.ultimaCompactacion,
		VersionMotor:       "EdgeSensorWave-Pebble-1.0",
	}
	
	return stats, nil
}

// Info retorna información detallada de la base de datos
func (db *DB) Info() (map[string]interface{}, error) {
	stats, err := db.Estadisticas()
	if err != nil {
		return nil, err
	}
	
	info := map[string]interface{}{
		"sensores":            stats.NumSensores,
		"registros":           stats.NumRegistros,
		"tamaño_bytes":        stats.TamañoBytes,
		"ultima_compactacion": stats.UltimaCompactacion,
		"version":             stats.VersionMotor,
		"opciones":            db.opciones,
	}
	
	return info, nil
}

// Sincronizar fuerza la sincronización de datos al disco
func (db *DB) Sincronizar() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	if db.cerrada {
		return fmt.Errorf("base de datos cerrada")
	}
	
	return db.pb.Flush()
}

// Compactar fuerza la compactación de la base de datos
func (db *DB) Compactar() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	if db.cerrada {
		return fmt.Errorf("base de datos cerrada")
	}
	
	// Compactar todo el rango
	if err := db.pb.Compact(nil, []byte{0xFF}, true); err != nil {
		return fmt.Errorf("error compactando: %w", err)
	}
	
	db.ultimaCompactacion = time.Now()
	return nil
}

// Cerrar cierra la base de datos
func (db *DB) Cerrar() error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	if db.cerrada {
		return nil
	}
	
	db.cerrada = true
	
	// Guardar estadísticas antes de cerrar
	_ = db.guardarEstadisticas()
	
	return db.pb.Close()
}

// cargarEstadisticas carga estadísticas persistidas
func (db *DB) cargarEstadisticas() error {
	// Contar sensores y registros existentes
	iter, err := db.pb.NewIter(nil)
	if err != nil {
		return err
	}
	defer iter.Close()
	
	sensores := make(map[string]bool)
	registros := int64(0)
	
	for iter.First(); iter.Valid(); iter.Next() {
		// Extraer ID de sensor de la clave
		if idSensor, err := ExtraerIDSensorDeClave(iter.Key()); err == nil {
			sensores[idSensor] = true
			registros++
		}
	}
	
	db.contadorSensores = sensores
	db.contadorRegistros = registros
	
	return iter.Error()
}

// guardarEstadisticas guarda estadísticas (placeholder para futura implementación)
func (db *DB) guardarEstadisticas() error {
	// Por ahora no persistimos estadísticas separadamente
	// En el futuro podríamos usar una clave especial en Pebble
	return nil
}

// validarIDSensor valida que un ID de sensor sea válido
func validarIDSensor(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("ID de sensor vacío")
	}
	
	if len(id) > 255 {
		return fmt.Errorf("ID de sensor demasiado largo: %d caracteres", len(id))
	}
	
	// Verificar caracteres válidos (letras, números, punto, guión bajo)
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || 
			 (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("carácter inválido en ID de sensor: %c", r)
		}
	}
	
	return nil
}