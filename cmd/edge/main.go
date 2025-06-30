package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/minio/minio-go/v7"
)

// BaseDatosIoTEmbebida - Base de datos embebida para IoT usando Pebble
type BaseDatosIoTEmbebida struct {
	// Almacenamiento local embebido con Pebble
	dbLocal             *pebble.DB
	rutaBaseDatos       string
	
	// Componentes de memoria
	bufferMemoria       *BufferMemoria
	indiceMemoria       *IndiceMemoria
	poolCompresion      *PoolCompresion
	
	// Sincronización con nube (opcional)
	sincronizadorNube   *SincronizadorNube
	configuracionNube   *ConfiguracionNube
	
	// Control interno
	mutex               sync.RWMutex
	activa              bool
	estadisticas        *EstadisticasDB
	configuracion       *ConfiguracionDB
	
	// Iteradores y snapshots de Pebble
	iteradores          map[string]*pebble.Iterator
	snapshots           map[string]*pebble.Snapshot
	mutexIteradores     sync.RWMutex
}

// ConfiguracionDB - Configuración optimizada para Pebble
type ConfiguracionDB struct {
	RutaBaseDatos       string        `json:"ruta_base_datos"`
	TamanoBufferMemoria int           `json:"tamano_buffer_memoria"`
	TiempoRetencionEdge time.Duration `json:"tiempo_retencion_edge"`
	CompresionHabilitada bool         `json:"compresion_habilitada"`
	SincronizacionNube  bool          `json:"sincronizacion_nube"`
	IntervaloSincronizacion time.Duration `json:"intervalo_sincronizacion"`
	MaxTamanoArchivo    int64         `json:"max_tamano_archivo"`
	
	// Configuraciones específicas de Pebble
	TamanoCache         int64         `json:"tamano_cache"`        // Cache de bloques
	TamanoMemtable      uint64        `json:"tamano_memtable"`     // Tamaño de memtable
	MaxNivelesCompactacion int        `json:"max_niveles_compactacion"`
	UsarBloomFilter     bool          `json:"usar_bloom_filter"`
	TamanoBloomFilter   int           `json:"tamano_bloom_filter"`
}

// RegistroSensor - Estructura principal para datos de sensores
type RegistroSensor struct {
	ID              string                 `json:"id"`
	TipoSensor      string                `json:"tipo_sensor"`
	MarcaTiempo     time.Time             `json:"marca_tiempo"`
	Valor           interface{}           `json:"valor"` // Puede ser float64, []float64, string, etc.
	Metadatos       map[string]interface{} `json:"metadatos"`
	Ubicacion       *Coordenadas          `json:"ubicacion,omitempty"`
	Calidad         CalidadDatos          `json:"calidad"`
	Comprimido      bool                  `json:"comprimido"`
	TamanoOriginal  int64                 `json:"tamano_original"`
	Version         int64                 `json:"version"` // Para versionado optimista
}

type Coordenadas struct {
	Latitud  float64 `json:"latitud"`
	Longitud float64 `json:"longitud"`
	Altitud  float64 `json:"altitud,omitempty"`
}

type CalidadDatos int

const (
	CalidadAlta CalidadDatos = iota
	CalidadMedia
	CalidadBaja
	CalidadDesconocida
)

// BufferMemoria - Buffer en memoria para acceso rápido
type BufferMemoria struct {
	registros       map[string]*RegistroSensor
	indicesTiempo   map[string][]time.Time // Por tipo de sensor
	capacidadMax    int
	mutex           sync.RWMutex
}

// IndiceMemoria - Índices para consultas rápidas
type IndiceMemoria struct {
	indiceTipoSensor    map[string][]string    // tipo_sensor -> []IDs
	indiceTiempo        map[int64][]string     // timestamp -> []IDs  
	indiceUbicacion     map[string][]string    // ubicacion -> []IDs
	indiceMetadatos     map[string]map[string][]string // metadato -> valor -> []IDs
	mutex               sync.RWMutex
}

// EstadisticasDB - Estadísticas mejoradas aprovechando métricas de Pebble
type EstadisticasDB struct {
	TotalRegistros         int64     `json:"total_registros"`
	RegistrosPorTipo       map[string]int64 `json:"registros_por_tipo"`
	TamanoBaseDatos        int64     `json:"tamano_base_datos"`
	UltimaActualizacion    time.Time `json:"ultima_actualizacion"`
	OperacionesLectura     int64     `json:"operaciones_lectura"`
	OperacionesEscritura   int64     `json:"operaciones_escritura"`
	TiempoPromedioConsulta time.Duration `json:"tiempo_promedio_consulta"`
	
	// Métricas específicas de Pebble
	MetricasPebble         *pebble.Metrics `json:"metricas_pebble,omitempty"`
	NivelesCompactacion    map[int]int64   `json:"niveles_compactacion"`
	HitRatioCache          float64         `json:"hit_ratio_cache"`
	
	mutex                  sync.RWMutex
}

// OpcionesConsulta - Opciones mejoradas para consultas con Pebble
type OpcionesConsulta struct {
	TiposSensor     []string               `json:"tipos_sensor,omitempty"`
	RangoTiempo     *RangoTiempo          `json:"rango_tiempo,omitempty"`
	Filtros         map[string]interface{} `json:"filtros,omitempty"`
	Limite          int                   `json:"limite,omitempty"`
	Orden           OrdenConsulta         `json:"orden,omitempty"`
	IncluirMetadatos bool                 `json:"incluir_metadatos"`
	SoloMemoria     bool                  `json:"solo_memoria"`
	
	// Opciones específicas de Pebble
	UsarSnapshot    bool                  `json:"usar_snapshot"`
	ConsistenciaFuerte bool              `json:"consistencia_fuerte"`
	PrefijoClave    string               `json:"prefijo_clave,omitempty"`
}

type RangoTiempo struct {
	Inicio time.Time `json:"inicio"`
	Fin    time.Time `json:"fin"`
}

type OrdenConsulta int

const (
	OrdenTiempoAsc OrdenConsulta = iota
	OrdenTiempoDesc
	OrdenTipoSensor
)

// ResultadoConsulta - Resultado de consultas
type ResultadoConsulta struct {
	Registros       []*RegistroSensor      `json:"registros"`
	TotalEncontrados int                   `json:"total_encontrados"`
	TiempoEjecucion time.Duration         `json:"tiempo_ejecucion"`
	FuenteDatos     string                `json:"fuente_datos"` // "memoria", "disco", "hibrido"
	Estadisticas    map[string]interface{} `json:"estadisticas,omitempty"`
	SnapshotUsado   bool                  `json:"snapshot_usado"`
}

// ================================
// CONSTRUCTOR Y INICIALIZACIÓN
// ================================

// NuevaBaseDatosIoTEmbebida - Constructor principal con Pebble
func NuevaBaseDatosIoTEmbebida(config *ConfiguracionDB) (*BaseDatosIoTEmbebida, error) {
	// Crear directorio si no existe
	if err := os.MkdirAll(config.RutaBaseDatos, 0755); err != nil {
		return nil, fmt.Errorf("error creando directorio: %v", err)
	}

	// Configurar opciones de Pebble optimizadas para IoT
	opcionesPebble := &pebble.Options{
		// Cache de bloques para lecturas rápidas
		Cache: pebble.NewCache(config.TamanoCache),
		
		// Configuración de memtable
		MemTableSize: config.TamanoMemtable,
		MemTableStopWritesThreshold: 4, // Hasta 4 memtables antes de bloquear escrituras
		
		// Configuración de compactación
		MaxOpenFiles:    1000,
		MaxConcurrentCompactions: func() int { return 3 }, // Función que retorna 3
		
		// Configuración de niveles
		Levels: configurarNivelesPebble(config.MaxNivelesCompactacion),
		
		// Compresión
		Comparer: pebble.DefaultComparer,
		
		// Logging
		Logger: pebble.DefaultLogger,
		
		// Configuración de WAL
		WALDir: filepath.Join(config.RutaBaseDatos, "wal"),
	}

	// Configurar Bloom Filter si está habilitado
	if config.UsarBloomFilter {
		// Configurar filtros por nivel
		for i := range opcionesPebble.Levels {
			opcionesPebble.Levels[i].FilterPolicy = bloom.FilterPolicy(config.TamanoBloomFilter)
		}
	}

	// Abrir base de datos Pebble
	dbLocal, err := pebble.Open(config.RutaBaseDatos, opcionesPebble)
	if err != nil {
		return nil, fmt.Errorf("error abriendo base de datos Pebble: %v", err)
	}

	bd := &BaseDatosIoTEmbebida{
		dbLocal:       dbLocal,
		rutaBaseDatos: config.RutaBaseDatos,
		configuracion: config,
		activa:        true,
		bufferMemoria: &BufferMemoria{
			registros:     make(map[string]*RegistroSensor),
			indicesTiempo: make(map[string][]time.Time),
			capacidadMax:  config.TamanoBufferMemoria,
		},
		indiceMemoria: &IndiceMemoria{
			indiceTipoSensor: make(map[string][]string),
			indiceTiempo:     make(map[int64][]string),
			indiceUbicacion:  make(map[string][]string),
			indiceMetadatos:  make(map[string]map[string][]string),
		},
		estadisticas: &EstadisticasDB{
			RegistrosPorTipo: make(map[string]int64),
			NivelesCompactacion: make(map[int]int64),
			UltimaActualizacion: time.Now(),
		},
		iteradores: make(map[string]*pebble.Iterator),
		snapshots:  make(map[string]*pebble.Snapshot),
	}

	// Inicializar pool de compresión si está habilitado
	if config.CompresionHabilitada {
		bd.poolCompresion = NuevoPoolCompresion(4) // 4 workers por defecto
	}

	// Cargar estadísticas de Pebble
	bd.actualizarMetricasPebble()

	// Cargar índices desde disco
	if err := bd.cargarIndicesDesdedisco(); err != nil {
		log.Printf("Advertencia: no se pudieron cargar índices: %v", err)
	}

	// Inicializar sincronización con nube si está configurada
	if config.SincronizacionNube {
		if err := bd.inicializarSincronizacionNube(); err != nil {
			log.Printf("Advertencia: no se pudo inicializar sincronización con nube: %v", err)
		}
	}

	// Iniciar rutinas de mantenimiento
	go bd.rutinMantenimiento()

	return bd, nil
}

// ================================
// API PRINCIPAL - OPERACIONES CRUD CON PEBBLE
// ================================

// Insertar - Inserta un nuevo registro usando Pebble
func (bd *BaseDatosIoTEmbebida) Insertar(registro *RegistroSensor) error {
	if !bd.activa {
		return fmt.Errorf("base de datos no está activa")
	}

	inicioTiempo := time.Now()
	defer func() {
		bd.estadisticas.mutex.Lock()
		bd.estadisticas.OperacionesEscritura++
		bd.estadisticas.UltimaActualizacion = time.Now()
		bd.estadisticas.mutex.Unlock()
	}()

	// Generar ID si no existe
	if registro.ID == "" {
		registro.ID = bd.generarID()
	}

	// Calcular tamaño original si no está definido
	if registro.TamanoOriginal == 0 {
		datos, err := json.Marshal(registro.Valor)
		if err != nil {
			return fmt.Errorf("error calculando tamaño: %v", err)
		}
		registro.TamanoOriginal = int64(len(datos))
	}

	// Asignar versión para control optimista
	registro.Version = time.Now().UnixNano()

	// Comprimir si está habilitado
	if bd.configuracion.CompresionHabilitada {
		if err := bd.comprimirRegistro(registro); err != nil {
			log.Printf("Error comprimiendo registro: %v", err)
		}
	}

	// Insertar en memoria si hay espacio
	bd.bufferMemoria.mutex.Lock()
	if len(bd.bufferMemoria.registros) < bd.bufferMemoria.capacidadMax {
		bd.bufferMemoria.registros[registro.ID] = registro
		bd.actualizarIndicesMemoria(registro)
	}
	bd.bufferMemoria.mutex.Unlock()

	// Persistir en Pebble con opciones de sincronización
	if err := bd.persistirRegistroPebble(registro); err != nil {
		return fmt.Errorf("error persistiendo registro: %v", err)
	}

	// Actualizar estadísticas
	bd.estadisticas.mutex.Lock()
	bd.estadisticas.TotalRegistros++
	bd.estadisticas.RegistrosPorTipo[registro.TipoSensor]++
	bd.estadisticas.mutex.Unlock()

	log.Printf("Registro insertado en %v", time.Since(inicioTiempo))
	return nil
}

// InsertarLote - Inserta múltiples registros usando batch de Pebble
func (bd *BaseDatosIoTEmbebida) InsertarLote(registros []*RegistroSensor) error {
	if !bd.activa {
		return fmt.Errorf("base de datos no está activa")
	}

	inicioTiempo := time.Now()
	
	// Usar batch de Pebble para mejor rendimiento
	batch := bd.dbLocal.NewBatch()
	defer batch.Close()
	
	for _, registro := range registros {
		if registro.ID == "" {
			registro.ID = bd.generarID()
		}

		// Asignar versión
		registro.Version = time.Now().UnixNano()

		// Comprimir si está habilitado
		if bd.configuracion.CompresionHabilitada {
			if err := bd.comprimirRegistro(registro); err != nil {
				log.Printf("Error comprimiendo registro %s: %v", registro.ID, err)
			}
		}

		// Serializar
		datos, err := json.Marshal(registro)
		if err != nil {
			return fmt.Errorf("error serializando registro %s: %v", registro.ID, err)
		}

		// Añadir al batch con clave prefijada
		clave := bd.generarClave(registro)
		if err := batch.Set([]byte(clave), datos, pebble.Sync); err != nil {
			return fmt.Errorf("error añadiendo al batch: %v", err)
		}

		// Actualizar memoria si hay espacio
		bd.bufferMemoria.mutex.Lock()
		if len(bd.bufferMemoria.registros) < bd.bufferMemoria.capacidadMax {
			bd.bufferMemoria.registros[registro.ID] = registro
			bd.actualizarIndicesMemoria(registro)
		}
		bd.bufferMemoria.mutex.Unlock()
	}

	// Confirmar batch con sincronización
	if err := batch.Commit(pebble.Sync); err != nil {
		return fmt.Errorf("error confirmando lote: %v", err)
	}

	// Actualizar estadísticas
	bd.estadisticas.mutex.Lock()
	bd.estadisticas.TotalRegistros += int64(len(registros))
	bd.estadisticas.OperacionesEscritura++
	for _, registro := range registros {
		bd.estadisticas.RegistrosPorTipo[registro.TipoSensor]++
	}
	bd.estadisticas.UltimaActualizacion = time.Now()
	bd.estadisticas.mutex.Unlock()

	log.Printf("Lote de %d registros insertado en %v", len(registros), time.Since(inicioTiempo))
	return nil
}

// Consultar - Consulta principal aprovechando capacidades de Pebble
func (bd *BaseDatosIoTEmbebida) Consultar(opciones *OpcionesConsulta) (*ResultadoConsulta, error) {
	if !bd.activa {
		return nil, fmt.Errorf("base de datos no está activa")
	}

	inicioTiempo := time.Now()
	defer func() {
		duracion := time.Since(inicioTiempo)
		bd.ActualizarEstadisticasConsulta(duracion)
		bd.estadisticas.mutex.Lock()
		bd.estadisticas.OperacionesLectura++
		bd.estadisticas.mutex.Unlock()
	}()

	resultado := &ResultadoConsulta{
		Registros: make([]*RegistroSensor, 0),
	}

	// Usar snapshot si se solicita consistencia fuerte
	var snapshot *pebble.Snapshot
	if opciones.UsarSnapshot || opciones.ConsistenciaFuerte {
		snapshot = bd.dbLocal.NewSnapshot()
		defer snapshot.Close()
		resultado.SnapshotUsado = true
	}

	// Consultar memoria primero
	registrosMemoria := bd.consultarMemoria(opciones)
	resultado.Registros = append(resultado.Registros, registrosMemoria...)

	// Si no es solo memoria, consultar Pebble
	if !opciones.SoloMemoria {
		registrosPebble, err := bd.consultarPebble(opciones, snapshot)
		if err != nil {
			return nil, fmt.Errorf("error consultando Pebble: %v", err)
		}
		resultado.Registros = append(resultado.Registros, registrosPebble...)
	}

	// Eliminar duplicados y aplicar límites
	resultado.Registros = bd.procesarResultados(resultado.Registros, opciones)
	
	resultado.TotalEncontrados = len(resultado.Registros)
	resultado.TiempoEjecucion = time.Since(inicioTiempo)
	
	// Determinar fuente de datos
	if len(registrosMemoria) > 0 && len(resultado.Registros) == len(registrosMemoria) {
		resultado.FuenteDatos = "memoria"
	} else if len(registrosMemoria) == 0 {
		resultado.FuenteDatos = "pebble"
	} else {
		resultado.FuenteDatos = "hibrido"
	}

	return resultado, nil
}

// ConsultarPorRango - Consulta optimizada por rango usando iteradores de Pebble
func (bd *BaseDatosIoTEmbebida) ConsultarPorRango(prefijo string, inicio, fin []byte, limite int) (*ResultadoConsulta, error) {
	if !bd.activa {
		return nil, fmt.Errorf("base de datos no está activa")
	}

	inicioTiempo := time.Now()
	
	resultado := &ResultadoConsulta{
		Registros: make([]*RegistroSensor, 0, limite),
	}

	// Crear iterador con opciones de rango
	opcionesIterador := &pebble.IterOptions{
		LowerBound: inicio,
		UpperBound: fin,
	}
	
	iterador, err := bd.dbLocal.NewIter(opcionesIterador)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %v", err)
	}
	defer iterador.Close()

	// Buscar desde el prefijo
	clavePrefijo := []byte(prefijo)
	count := 0
	
	for iterador.SeekGE(clavePrefijo); iterador.Valid() && count < limite; iterador.Next() {
		// Verificar que la clave sigue teniendo el prefijo correcto
		clave := iterador.Key()
		if len(clave) < len(clavePrefijo) || !bytes.HasPrefix(clave, clavePrefijo) {
			break
		}

		// Deserializar registro
		var registro RegistroSensor
		if err := json.Unmarshal(iterador.Value(), &registro); err != nil {
			log.Printf("Error deserializando registro: %v", err)
			continue
		}

		resultado.Registros = append(resultado.Registros, &registro)
		count++
	}

	// Verificar errores del iterador
	if err := iterador.Error(); err != nil {
		return nil, fmt.Errorf("error en iterador: %v", err)
	}

	resultado.TotalEncontrados = len(resultado.Registros)
	resultado.TiempoEjecucion = time.Since(inicioTiempo)
	resultado.FuenteDatos = "pebble"

	return resultado, nil
}

// ================================
// MÉTODOS ESPECÍFICOS DE PEBBLE
// ================================

// CompactarRango - Compacta un rango específico en Pebble
func (bd *BaseDatosIoTEmbebida) CompactarRango(inicio, fin []byte) error {
	if !bd.activa {
		return fmt.Errorf("base de datos no está activa")
	}

	log.Printf("Iniciando compactación de rango...")
	inicioTiempo := time.Now()
	
	// Usar compactación manual de Pebble
	err := bd.dbLocal.Compact(inicio, fin, true) // true = paralelización
	if err != nil {
		return fmt.Errorf("error compactando rango: %v", err)
	}

	log.Printf("Compactación de rango completada en %v", time.Since(inicioTiempo))
	return nil
}

// ObtenerMetricasPebble - Obtiene métricas detalladas de Pebble
func (bd *BaseDatosIoTEmbebida) ObtenerMetricasPebble() *pebble.Metrics {
	if !bd.activa {
		return nil
	}

	metricas := bd.dbLocal.Metrics()
	
	// Actualizar estadísticas internas
	bd.estadisticas.mutex.Lock()
	bd.estadisticas.MetricasPebble = metricas
	
	// Calcular hit ratio del cache
	if metricas.BlockCache.Hits+metricas.BlockCache.Misses > 0 {
		bd.estadisticas.HitRatioCache = float64(metricas.BlockCache.Hits) / 
			float64(metricas.BlockCache.Hits+metricas.BlockCache.Misses)
	}
	
	// Actualizar información de niveles
	for i, nivel := range metricas.Levels {
		bd.estadisticas.NivelesCompactacion[i] = nivel.Size
	}
	
	bd.estadisticas.mutex.Unlock()

	return metricas
}

// CrearSnapshot - Crea un snapshot para lecturas consistentes
func (bd *BaseDatosIoTEmbebida) CrearSnapshot(id string) error {
	if !bd.activa {
		return fmt.Errorf("base de datos no está activa")
	}

	bd.mutexIteradores.Lock()
	defer bd.mutexIteradores.Unlock()

	// Cerrar snapshot existente si existe
	if snapshot, existe := bd.snapshots[id]; existe {
		if err := snapshot.Close(); err != nil {
			log.Printf("Error cerrando snapshot existente '%s': %v", id, err)
		}
	}

	// Crear nuevo snapshot
	snapshot := bd.dbLocal.NewSnapshot()
	bd.snapshots[id] = snapshot
	
	log.Printf("Snapshot '%s' creado", id)
	return nil
}

// LiberarSnapshot - Libera un snapshot
func (bd *BaseDatosIoTEmbebida) LiberarSnapshot(id string) error {
	bd.mutexIteradores.Lock()
	defer bd.mutexIteradores.Unlock()

	if snapshot, existe := bd.snapshots[id]; existe {
		if err := snapshot.Close(); err != nil {
			return fmt.Errorf("error cerrando snapshot '%s': %v", id, err)
		}
		delete(bd.snapshots, id)
		log.Printf("Snapshot '%s' liberado", id)
		return nil
	}

	return fmt.Errorf("snapshot '%s' no encontrado", id)
}

// ================================
// MÉTODOS INTERNOS OPTIMIZADOS
// ================================

func (bd *BaseDatosIoTEmbebida) persistirRegistroPebble(registro *RegistroSensor) error {
	datos, err := json.Marshal(registro)
	if err != nil {
		return err
	}

	clave := bd.generarClave(registro)
	
	// Usar opciones de escritura optimizadas
	opcionesEscritura := pebble.Sync // Sincronización por defecto
	if bd.configuracion.TamanoBufferMemoria > 5000 {
		opcionesEscritura = pebble.NoSync // Para alto volumen, sync manual periódico
	}
	
	return bd.dbLocal.Set([]byte(clave), datos, opcionesEscritura)
}

func (bd *BaseDatosIoTEmbebida) consultarPebble(opciones *OpcionesConsulta, snapshot *pebble.Snapshot) ([]*RegistroSensor, error) {
	registros := make([]*RegistroSensor, 0)

	// Configurar iterador
	var iterador *pebble.Iterator
	var err error
	
	if snapshot != nil {
		iterador, err = snapshot.NewIter(nil)
		if err != nil {
			return nil, fmt.Errorf("error creando iterador de snapshot: %v", err)
		}
	} else {
		iterador, err = bd.dbLocal.NewIter(nil)
		if err != nil {
			return nil, fmt.Errorf("error creando iterador: %v", err)
		}
	}
	defer iterador.Close()

	// Si hay prefijo de clave, optimizar búsqueda
	var claveInicio []byte
	if opciones.PrefijoClave != "" {
		claveInicio = []byte(opciones.PrefijoClave)
	} else {
		claveInicio = []byte("registro:")
	}

	count := 0
	limite := opciones.Limite
	if limite == 0 {
		limite = 1000 // Límite por defecto
	}

	for iterador.SeekGE(claveInicio); iterador.Valid() && count < limite; iterador.Next() {
		var registro RegistroSensor
		if err := json.Unmarshal(iterador.Value(), &registro); err != nil {
			log.Printf("Error deserializando registro: %v", err)
			continue
		}

		// Aplicar filtros
		if bd.aplicarFiltros(&registro, opciones) {
			registros = append(registros, &registro)
			count++
		}
	}

	if err := iterador.Error(); err != nil {
		return nil, fmt.Errorf("error en iterador Pebble: %v", err)
	}

	return registros, nil
}

func (bd *BaseDatosIoTEmbebida) actualizarMetricasPebble() {
	bd.estadisticas.mutex.Lock()
	defer bd.estadisticas.mutex.Unlock()
	
	metricas := bd.dbLocal.Metrics()
	bd.estadisticas.MetricasPebble = metricas
	
	// Calcular tamaño total de la base de datos
	var tamanoTotal int64
	for _, nivel := range metricas.Levels {
		tamanoTotal += nivel.Size
	}
	bd.estadisticas.TamanoBaseDatos = tamanoTotal
}

func (bd *BaseDatosIoTEmbebida) rutinMantenimiento() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Actualizar métricas de Pebble
			bd.actualizarMetricasPebble()
			
			// Limpiar memoria si es necesario
			bd.LimpiarMemoria()
			
			// Compactación automática si es necesario
			metricas := bd.ObtenerMetricasPebble()
			if metricas != nil && bd.necesitaCompactacion(metricas) {
				go bd.CompactarBaseDatos()
			}
		}
	}
}

func (bd *BaseDatosIoTEmbebida) necesitaCompactacion(metricas *pebble.Metrics) bool {
	// Heurística simple: compactar si hay muchos niveles con datos
	nivelesConDatos := 0
	for _, nivel := range metricas.Levels {
		if nivel.Size > 0 {
			nivelesConDatos++
		}
	}
	return nivelesConDatos > 4
}

// ConfiguracionPorDefectoPebble - Configuración optimizada para Pebble
func ConfiguracionPorDefectoPebble() *ConfiguracionDB {
	return &ConfiguracionDB{
		RutaBaseDatos:           "./datos_iot_pebble.db",
		TamanoBufferMemoria:     10000,
		TiempoRetencionEdge:     time.Hour * 24,
		CompresionHabilitada:    true,
		SincronizacionNube:      false,
		IntervaloSincronizacion: time.Minute * 30,
		MaxTamanoArchivo:        1024 * 1024 * 1024, // 1GB
		
		// Configuraciones específicas de Pebble
		TamanoCache:             128 * 1024 * 1024, // 128MB cache
		TamanoMemtable:          64 * 1024 * 1024,  // 64MB memtable
		MaxNivelesCompactacion:  7,                 // 7 niveles LSM
		UsarBloomFilter:         true,
		TamanoBloomFilter:       10,                // 10 bits por clave
	}
}

// ObtenerEstadisticas - Retorna estadísticas de la base de datos
func (bd *BaseDatosIoTEmbebida) ObtenerEstadisticas() *EstadisticasDB {
	bd.estadisticas.mutex.RLock()
	defer bd.estadisticas.mutex.RUnlock()
	
	// Crear una copia de las estadísticas para evitar modificaciones concurrentes
	estadisticasCopia := &EstadisticasDB{
		TotalRegistros:         bd.estadisticas.TotalRegistros,
		RegistrosPorTipo:       make(map[string]int64),
		TamanoBaseDatos:        bd.estadisticas.TamanoBaseDatos,
		UltimaActualizacion:    bd.estadisticas.UltimaActualizacion,
		OperacionesLectura:     bd.estadisticas.OperacionesLectura,
		OperacionesEscritura:   bd.estadisticas.OperacionesEscritura,
		TiempoPromedioConsulta: bd.estadisticas.TiempoPromedioConsulta,
		NivelesCompactacion:    make(map[int]int64),
		HitRatioCache:          bd.estadisticas.HitRatioCache,
	}
	
	// Copiar mapas para evitar referencias compartidas
	for tipo, cantidad := range bd.estadisticas.RegistrosPorTipo {
		estadisticasCopia.RegistrosPorTipo[tipo] = cantidad
	}
	
	for nivel, tamano := range bd.estadisticas.NivelesCompactacion {
		estadisticasCopia.NivelesCompactacion[nivel] = tamano
	}
	
	// Copiar métricas de Pebble si existen
	if bd.estadisticas.MetricasPebble != nil {
		metricasCopia := *bd.estadisticas.MetricasPebble
		estadisticasCopia.MetricasPebble = &metricasCopia
	}
	
	return estadisticasCopia
}

// ActualizarEstadisticasConsulta - Actualiza estadísticas de consulta
func (bd *BaseDatosIoTEmbebida) ActualizarEstadisticasConsulta(duracion time.Duration) {
	bd.estadisticas.mutex.Lock()
	defer bd.estadisticas.mutex.Unlock()
	
	// Calcular promedio móvil simple del tiempo de consulta
	if bd.estadisticas.TiempoPromedioConsulta == 0 {
		bd.estadisticas.TiempoPromedioConsulta = duracion
	} else {
		// Promedio ponderado: 80% valor anterior, 20% nuevo valor
		bd.estadisticas.TiempoPromedioConsulta = 
			time.Duration(float64(bd.estadisticas.TiempoPromedioConsulta)*0.8 + 
			float64(duracion)*0.2)
	}
}

func configurarNivelesPebble(maxNiveles int) []pebble.LevelOptions {
	niveles := make([]pebble.LevelOptions, maxNiveles)
	
	for i := 0; i < maxNiveles; i++ {
		niveles[i] = pebble.LevelOptions{
			BlockSize:      32 * 1024, // 32KB bloques
			IndexBlockSize: 4 * 1024,  // 4KB índices
			FilterType:     pebble.TableFilter,
			Compression:    pebble.SnappyCompression, // Compresión Snappy por defecto
		}
		
		// Niveles superiores usan compresión más agresiva
		if i >= 2 {
			niveles[i].Compression = pebble.ZstdCompression
		}
	}
	
	return niveles
}

// ================================
// MÉTODOS AUXILIARES COMPLETADOS
// ================================

func (bd *BaseDatosIoTEmbebida) generarID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), bd.estadisticas.TotalRegistros)
}

func (bd *BaseDatosIoTEmbebida) generarClave(registro *RegistroSensor) string {
	// Usar prefijo para organizar claves por tipo de sensor
	return fmt.Sprintf("registro:%s:%s:%d", registro.TipoSensor, registro.ID, registro.MarcaTiempo.Unix())
}

func (bd *BaseDatosIoTEmbebida) comprimirRegistro(registro *RegistroSensor) error {
	// Calcular tamaño original si no se ha calculado
	if registro.TamanoOriginal == 0 {
		datos, err := json.Marshal(registro.Valor)
		if err != nil {
			return fmt.Errorf("error calculando tamaño original: %v", err)
		}
		registro.TamanoOriginal = int64(len(datos))
	}
	
	// Implementar compresión según el tipo de datos
	// Por ahora solo marcamos como comprimido
	// Aquí se puede implementar compresión real (gzip, snappy, etc.)
	registro.Comprimido = true
	return nil
}

func (bd *BaseDatosIoTEmbebida) actualizarIndicesMemoria(registro *RegistroSensor) {
	bd.indiceMemoria.mutex.Lock()
	defer bd.indiceMemoria.mutex.Unlock()

	// Índice por tipo de sensor
	bd.indiceMemoria.indiceTipoSensor[registro.TipoSensor] = append(
		bd.indiceMemoria.indiceTipoSensor[registro.TipoSensor], registro.ID)

	// Índice por tiempo (por hora)
	timestamp := registro.MarcaTiempo.Truncate(time.Hour).Unix()
	bd.indiceMemoria.indiceTiempo[timestamp] = append(
		bd.indiceMemoria.indiceTiempo[timestamp], registro.ID)

	// Índice por ubicación si existe
	if registro.Ubicacion != nil {
		ubicacionKey := fmt.Sprintf("%.2f,%.2f", registro.Ubicacion.Latitud, registro.Ubicacion.Longitud)
		bd.indiceMemoria.indiceUbicacion[ubicacionKey] = append(
			bd.indiceMemoria.indiceUbicacion[ubicacionKey], registro.ID)
	}
}

func (bd *BaseDatosIoTEmbebida) consultarMemoria(opciones *OpcionesConsulta) []*RegistroSensor {
	bd.bufferMemoria.mutex.RLock()
	defer bd.bufferMemoria.mutex.RUnlock()

	registros := make([]*RegistroSensor, 0)
	
	for _, registro := range bd.bufferMemoria.registros {
		if bd.aplicarFiltros(registro, opciones) {
			registros = append(registros, registro)
		}
	}

	return registros
}

func (bd *BaseDatosIoTEmbebida) aplicarFiltros(registro *RegistroSensor, opciones *OpcionesConsulta) bool {
	// Filtro por tipo de sensor
	if len(opciones.TiposSensor) > 0 {
		encontrado := false
		for _, tipo := range opciones.TiposSensor {
			if registro.TipoSensor == tipo {
				encontrado = true
				break
			}
		}
		if !encontrado {
			return false
		}
	}

	// Filtro por rango de tiempo
	if opciones.RangoTiempo != nil {
		if registro.MarcaTiempo.Before(opciones.RangoTiempo.Inicio) || 
		   registro.MarcaTiempo.After(opciones.RangoTiempo.Fin) {
			return false
		}
	}

	// Filtros adicionales en metadatos
	for clave, valor := range opciones.Filtros {
		if metadatoValor, existe := registro.Metadatos[clave]; !existe || metadatoValor != valor {
			return false
		}
	}

	return true
}

func (bd *BaseDatosIoTEmbebida) procesarResultados(registros []*RegistroSensor, opciones *OpcionesConsulta) []*RegistroSensor {
	// Eliminar duplicados por ID
	mapaRegistros := make(map[string]*RegistroSensor)
	for _, registro := range registros {
		if existente, existe := mapaRegistros[registro.ID]; !existe || registro.Version > existente.Version {
			mapaRegistros[registro.ID] = registro
		}
	}

	// Convertir mapa a slice
	resultados := make([]*RegistroSensor, 0, len(mapaRegistros))
	for _, registro := range mapaRegistros {
		resultados = append(resultados, registro)
	}

	// Ordenar según opciones
	bd.ordenarResultados(resultados, opciones.Orden)

	// Aplicar límite
	if opciones.Limite > 0 && len(resultados) > opciones.Limite {
		resultados = resultados[:opciones.Limite]
	}

	return resultados
}

func (bd *BaseDatosIoTEmbebida) ordenarResultados(registros []*RegistroSensor, orden OrdenConsulta) {
	switch orden {
	case OrdenTiempoAsc:
		// Ordenar por tiempo ascendente
		for i := 0; i < len(registros)-1; i++ {
			for j := i + 1; j < len(registros); j++ {
				if registros[i].MarcaTiempo.After(registros[j].MarcaTiempo) {
					registros[i], registros[j] = registros[j], registros[i]
				}
			}
		}
	case OrdenTiempoDesc:
		// Ordenar por tiempo descendente
		for i := 0; i < len(registros)-1; i++ {
			for j := i + 1; j < len(registros); j++ {
				if registros[i].MarcaTiempo.Before(registros[j].MarcaTiempo) {
					registros[i], registros[j] = registros[j], registros[i]
				}
			}
		}
	case OrdenTipoSensor:
		// Ordenar por tipo de sensor
		for i := 0; i < len(registros)-1; i++ {
			for j := i + 1; j < len(registros); j++ {
				if registros[i].TipoSensor > registros[j].TipoSensor {
					registros[i], registros[j] = registros[j], registros[i]
				}
			}
		}
	}
}

func (bd *BaseDatosIoTEmbebida) cargarIndicesDesdedisco() error {
	// Implementar carga de índices desde archivos auxiliares
	log.Println("Cargando índices desde disco...")
	return nil
}

func (bd *BaseDatosIoTEmbebida) guardarIndicesEnDisco() error {
	// Implementar guardado de índices en archivos auxiliares
	log.Println("Guardando índices en disco...")
	return nil
}

func (bd *BaseDatosIoTEmbebida) inicializarSincronizacionNube() error {
	// Implementar sincronización con nube
	log.Println("Inicializando sincronización con nube...")
	return nil
}

func (bd *BaseDatosIoTEmbebida) tieneRangoCompleto(rangoTiempo *RangoTiempo) bool {
	// Verificar si el rango de tiempo está completamente en memoria
	if rangoTiempo == nil {
		return true
	}
	
	bd.bufferMemoria.mutex.RLock()
	defer bd.bufferMemoria.mutex.RUnlock()
	
	for _, registro := range bd.bufferMemoria.registros {
		if registro.MarcaTiempo.Before(rangoTiempo.Inicio) {
			return false
		}
	}
	return true
}

// ConsultarPorID - Consulta optimizada por ID usando Pebble
func (bd *BaseDatosIoTEmbebida) ConsultarPorID(id string) (*RegistroSensor, error) {
	if !bd.activa {
		return nil, fmt.Errorf("base de datos no está activa")
	}

	// Buscar primero en memoria
	bd.bufferMemoria.mutex.RLock()
	if registro, existe := bd.bufferMemoria.registros[id]; existe {
		bd.bufferMemoria.mutex.RUnlock()
		return registro, nil
	}
	bd.bufferMemoria.mutex.RUnlock()

	// Buscar en Pebble usando prefijo optimizado
	// Necesitamos iterar porque la clave incluye tipo y timestamp
	iterador, err := bd.dbLocal.NewIter(nil)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %v", err)
	}
	defer iterador.Close()

	prefijoRegistro := []byte("registro:")
	for iterador.SeekGE(prefijoRegistro); iterador.Valid(); iterador.Next() {
		var registro RegistroSensor
		if err := json.Unmarshal(iterador.Value(), &registro); err != nil {
			continue
		}
		
		if registro.ID == id {
			return &registro, nil
		}
	}

	if err := iterador.Error(); err != nil {
		return nil, fmt.Errorf("error consultando registro: %v", err)
	}

	return nil, fmt.Errorf("registro no encontrado: %s", id)
}

// CompactarBaseDatos - Compacta toda la base de datos Pebble
func (bd *BaseDatosIoTEmbebida) CompactarBaseDatos() error {
	if !bd.activa {
		return fmt.Errorf("base de datos no está activa")
	}

	log.Println("Iniciando compactación completa de base de datos...")
	inicio := time.Now()
	
	// Compactar todo el rango
	err := bd.dbLocal.Compact(nil, nil, true)
	if err != nil {
		return fmt.Errorf("error compactando base de datos: %v", err)
	}

	log.Printf("Compactación completa en %v", time.Since(inicio))
	return nil
}

// LimpiarMemoria - Limpia el buffer de memoria según políticas
func (bd *BaseDatosIoTEmbebida) LimpiarMemoria() {
	bd.bufferMemoria.mutex.Lock()
	defer bd.bufferMemoria.mutex.Unlock()

	// Limpiar registros más antiguos si excede capacidad
	if len(bd.bufferMemoria.registros) > bd.bufferMemoria.capacidadMax {
		registrosAEliminar := len(bd.bufferMemoria.registros) - bd.bufferMemoria.capacidadMax
		
		// Crear slice de registros ordenados por tiempo
		type registroConTiempo struct {
			id     string
			tiempo time.Time
		}
		
		registrosOrdenados := make([]registroConTiempo, 0, len(bd.bufferMemoria.registros))
		for id, registro := range bd.bufferMemoria.registros {
			registrosOrdenados = append(registrosOrdenados, registroConTiempo{
				id:     id,
				tiempo: registro.MarcaTiempo,
			})
		}

		// Ordenar por tiempo (más antiguos primero)
		for i := 0; i < len(registrosOrdenados)-1; i++ {
			for j := i + 1; j < len(registrosOrdenados); j++ {
				if registrosOrdenados[i].tiempo.After(registrosOrdenados[j].tiempo) {
					registrosOrdenados[i], registrosOrdenados[j] = registrosOrdenados[j], registrosOrdenados[i]
				}
			}
		}

		// Eliminar los más antiguos
		for i := 0; i < registrosAEliminar && i < len(registrosOrdenados); i++ {
			delete(bd.bufferMemoria.registros, registrosOrdenados[i].id)
		}
		
		log.Printf("Limpiados %d registros de memoria", registrosAEliminar)
	}
}

// Cerrar - Cierra la base de datos Pebble de forma segura
func (bd *BaseDatosIoTEmbebida) Cerrar() error {
	bd.mutex.Lock()
	defer bd.mutex.Unlock()

	if !bd.activa {
		return nil
	}

	log.Println("Cerrando base de datos Pebble...")

	// Cerrar todos los snapshots
	bd.mutexIteradores.Lock()
	for id, snapshot := range bd.snapshots {
		if err := snapshot.Close(); err != nil {
			log.Printf("Error cerrando snapshot '%s': %v", id, err)
		} else {
			log.Printf("Snapshot '%s' cerrado", id)
		}
	}
	bd.snapshots = make(map[string]*pebble.Snapshot)
	bd.mutexIteradores.Unlock()

	// Guardar índices en disco
	if err := bd.guardarIndicesEnDisco(); err != nil {
		log.Printf("Error guardando índices: %v", err)
	}

	// Cerrar base de datos Pebble
	if err := bd.dbLocal.Close(); err != nil {
		return fmt.Errorf("error cerrando base de datos Pebble: %v", err)
	}

	bd.activa = false
	log.Println("Base de datos Pebble cerrada correctamente")
	return nil
}

// ================================
// TIPOS AUXILIARES PARA COMPRESIÓN
// ================================

type PoolCompresion struct {
	trabajadores []*TrabajadorCompresion
	colaTareas   chan TareaCompresion
	mutex        sync.RWMutex
}

type TareaCompresion struct {
	Datos    []byte
	Callback func([]byte, error)
}

type TrabajadorCompresion struct {
	id   int
	pool *PoolCompresion
}

func NuevoPoolCompresion(numTrabajadores int) *PoolCompresion {
	pool := &PoolCompresion{
		trabajadores: make([]*TrabajadorCompresion, numTrabajadores),
		colaTareas:   make(chan TareaCompresion, numTrabajadores*10),
	}
	
	for i := 0; i < numTrabajadores; i++ {
		trabajador := &TrabajadorCompresion{id: i, pool: pool}
		pool.trabajadores[i] = trabajador
		go trabajador.iniciar()
	}
	
	return pool
}

func (p *PoolCompresion) Enviar(tarea TareaCompresion) {
	select {
	case p.colaTareas <- tarea:
	default:
		// Cola llena, ejecutar síncronamente
		tarea.Callback(tarea.Datos, nil)
	}
}

func (t *TrabajadorCompresion) iniciar() {
	for tarea := range t.pool.colaTareas {
		// Simular compresión - implementar algoritmos reales aquí
		tarea.Callback(tarea.Datos, nil)
	}
}

// ================================
// TIPOS PARA SINCRONIZACIÓN NUBE
// ================================

type SincronizadorNube struct {
	cliente *minio.Client
	config  *ConfiguracionNube
}

type ConfiguracionNube struct {
	Endpoint     string `json:"endpoint"`
	ClaveAcceso  string `json:"clave_acceso"`
	ClaveSecreta string `json:"clave_secreta"`
	Bucket       string `json:"bucket"`
	UsarSSL      bool   `json:"usar_ssl"`
}

// ================================
// EJEMPLO DE USO COMPLETO
// ================================

func EjemploUsoCompleto() {
	// Crear configuración optimizada para Pebble
	config := ConfiguracionPorDefectoPebble()
	config.RutaBaseDatos = "./datos_iot_pebble_ejemplo.db"
	config.TamanoCache = 256 * 1024 * 1024 // 256MB cache
	config.TamanoMemtable = 128 * 1024 * 1024 // 128MB memtable

	// Inicializar base de datos
	bd, err := NuevaBaseDatosIoTEmbebida(config)
	if err != nil {
		log.Fatalf("Error inicializando BD: %v", err)
	}
	defer bd.Cerrar()

	// Insertar datos de ejemplo
	registros := []*RegistroSensor{
		{
			TipoSensor:  "temperatura",
			MarcaTiempo: time.Now(),
			Valor:       23.5,
			Metadatos: map[string]interface{}{
				"ubicacion": "sala_1",
				"unidad":    "celsius",
			},
			Ubicacion: &Coordenadas{Latitud: -34.6037, Longitud: -58.3816},
			Calidad:   CalidadAlta,
		},
		{
			TipoSensor:  "humedad",
			MarcaTiempo: time.Now(),
			Valor:       65.2,
			Metadatos: map[string]interface{}{
				"ubicacion": "sala_1",
				"unidad":    "porcentaje",
			},
			Ubicacion: &Coordenadas{Latitud: -34.6037, Longitud: -58.3816},
			Calidad:   CalidadAlta,
		},
	}

	// Insertar lote
	if err := bd.InsertarLote(registros); err != nil {
		log.Printf("Error insertando lote: %v", err)
	}

	// Consultar con snapshot para consistencia
	opciones := &OpcionesConsulta{
		TiposSensor: []string{"temperatura", "humedad"},
		RangoTiempo: &RangoTiempo{
			Inicio: time.Now().Add(-time.Hour),
			Fin:    time.Now(),
		},
		Limite:             100,
		UsarSnapshot:       true,
		ConsistenciaFuerte: true,
		IncluirMetadatos:   true,
	}

	resultado, err := bd.Consultar(opciones)
	if err != nil {
		log.Printf("Error consultando: %v", err)
	} else {
		for _, registro := range resultado.Registros {
			fmt.Printf("Registro: %s, Tipo: %s, Valor: %v, MarcaTiempo: %s\n",
				registro.ID, registro.TipoSensor, registro.Valor, registro.MarcaTiempo)
			if registro.Ubicacion != nil {
				fmt.Printf("Ubicación: Latitud %.2f, Longitud %.2f\n",
					registro.Ubicacion.Latitud, registro.Ubicacion.Longitud)
			}
			if registro.Metadatos != nil {
				fmt.Printf("Metadatos: %v\n", registro.Metadatos)
			}
			fmt.Printf("Calidad: %v\n", registro.Calidad)
			fmt.Println("-----")
		}

		fmt.Printf("Encontrados %d registros en %v\n", 
			resultado.TotalEncontrados, resultado.TiempoEjecucion)
		fmt.Printf("Fuente de datos: %s\n", resultado.FuenteDatos)
		fmt.Printf("Snapshot usado: %t\n", resultado.SnapshotUsado)
	}

	// Obtener métricas de Pebble
	metricas := bd.ObtenerMetricasPebble()
	if metricas != nil {
		fmt.Printf("Cache hit ratio: %.2f%%\n", 
			float64(metricas.BlockCache.Hits)/(float64(metricas.BlockCache.Hits+metricas.BlockCache.Misses))*100)
		fmt.Printf("Niveles LSM: %d\n", len(metricas.Levels))
	}

	// Estadísticas generales
	estadisticas := bd.ObtenerEstadisticas()
	fmt.Printf("Total registros: %d\n", estadisticas.TotalRegistros)
	fmt.Printf("Operaciones de lectura: %d\n", estadisticas.OperacionesLectura)
	fmt.Printf("Operaciones de escritura: %d\n", estadisticas.OperacionesEscritura)
}

func main() {
	EjemploUsoCompleto()
}