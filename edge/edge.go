package edge

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_nats"
	"github.com/cockroachdb/pebble"
	"github.com/google/uuid"
)

type ManagerEdge struct {
	db              *pebble.DB              // Conexión a PebbleDB
	cache           *Cache                  // Cache para configuraciones de series
	buffers         sync.Map                // Buffers por serie (thread-safe map)
	done            chan struct{}           // Canal para señalar cierre
	counter         int                     // Contador para serie_id
	mu              sync.RWMutex            // Mutex para proteger counter
	motorReglas     *MotorReglas            // Motor de reglas integrado
	nodeID          string                  // ID único persistente del nodo
	cliente         *sw_cliente.ClienteNATS // Cliente NATS
	direccionNATS   string                  // Dirección del servidor NATS
	puertoNATS      string                  // Puerto del servidor NATS
	reconectando    bool                    // Indica si está en proceso de reconexión
	muConexion      sync.Mutex              // Mutex para proteger operaciones de conexión
	ultimaSincro    time.Time               // Timestamp de última sincronización
	muSincronizando sync.Mutex              // Mutex para evitar sincronizaciones concurrentes
}

type Cache struct {
	datos map[string]Serie // Mapa para almacenar las configuraciones de series
	mu    sync.RWMutex     // Mutex para proteger el acceso concurrente
}

type Serie struct {
	SerieId          int                   // ID de la serie en la base de datos
	NombreSerie      string                // Nombre de la serie (deprecated, usar Path)
	Path             string                // Path jerárquico: "dispositivo_001/temperatura"
	Tags             map[string]string     // Tags: {"ubicacion": "sala1", "tipo": "DHT22"}
	TipoDatos        TipoDatos             // Tipo de datos almacenados
	CompresionBloque TipoCompresionBloque  // Compresión nivel bloque
	CompresionBytes  TipoCompresionValores // Compresión nivel valores
	TamañoBloque     int                   // Tamaño del bloque
}

type TipoCompresionBloque string

const (
	Ninguna TipoCompresionBloque = "Ninguna"
	LZ4     TipoCompresionBloque = "LZ4"
	ZSTD    TipoCompresionBloque = "ZSTD"
	Snappy  TipoCompresionBloque = "Snappy"
	Gzip    TipoCompresionBloque = "Gzip"
)

type TipoCompresionValores string

const (
	SinCompresion TipoCompresionValores = "SinCompresion"
	DeltaDelta    TipoCompresionValores = "DeltaDelta"
	RLE           TipoCompresionValores = "RLE"
	Bits          TipoCompresionValores = "Bits"
)

type TipoDatos string

const (
	TipoNumerico   TipoDatos = "NUMERICO"
	TipoCategorico TipoDatos = "CATEGORICO"
	TipoMixto      TipoDatos = "MIXTO"
)

type Medicion struct {
	Tiempo int64
	Valor  interface{}
}

type SerieBuffer struct {
	datos      []Medicion    // Arreglo con TamañoBloque elementos
	serie      Serie         // Configuración de la serie
	indice     int           // Índice actual en el buffer
	mu         sync.Mutex    // Mutex para proteger el buffer
	done       chan struct{} // Canal para señalar cierre del hilo
	datosCanal chan Medicion // Canal para recibir nuevos datos
}

type SuscripcionNodo struct {
	ID     string            `json:"id"`
	Series map[string]string `json:"series"`
}

type NuevaSerie struct {
	NodeID  string `json:"node_id"`
	Path    string `json:"path"`
	SerieID int    `json:"serie_id"`
}

type Heartbeat struct {
	NodeID    string    `json:"node_id"`
	Timestamp time.Time `json:"timestamp"`
	Activo    bool      `json:"activo"`
}

// Crear inicializa el ManagerEdge con PebbleDB y la cache
func Crear(nombre string, direccionNATS string, puertoNATS string) (ManagerEdge, error) {
	db, err := pebble.Open(nombre, &pebble.Options{})
	if err != nil {
		return ManagerEdge{}, err
	}

	// Crear el manager con PebbleDB
	manager := &ManagerEdge{
		db: db,
		cache: &Cache{
			datos: make(map[string]Serie),
		},
		done:          make(chan struct{}),
		counter:       0,
		direccionNATS: direccionNATS,
		puertoNATS:    puertoNATS,
		reconectando:  false,
		ultimaSincro:  time.Time{},
	}

	// Cargar o generar nodeID
	nodeIDBytes, closer, err := db.Get([]byte("meta/node_id"))
	if err == pebble.ErrNotFound {
		manager.nodeID = generarNodeID()
		err = db.Set([]byte("meta/node_id"), []byte(manager.nodeID), pebble.Sync)
		if err != nil {
			return ManagerEdge{}, fmt.Errorf("error al guardar node_id: %v", err)
		}
		log.Printf("Nuevo nodeID generado: %s", manager.nodeID)
	} else if err != nil {
		return ManagerEdge{}, fmt.Errorf("error al leer node_id: %v", err)
	} else {
		manager.nodeID = string(nodeIDBytes)
		closer.Close()
		log.Printf("NodeID cargado desde DB: %s", manager.nodeID)
	}

	// Cargar contador de series desde PebbleDB
	counterBytes, closer, err := db.Get([]byte("meta/counter"))
	if err != nil && err != pebble.ErrNotFound {
		return ManagerEdge{}, fmt.Errorf("error al leer contador: %v", err)
	}
	if err == nil {
		if len(counterBytes) >= 4 {
			manager.counter = int(binary.LittleEndian.Uint32(counterBytes))
		}
		closer.Close()
	}

	// Cargar series existentes desde PebbleDB
	err = manager.cargarSeriesExistentes()
	if err != nil {
		return ManagerEdge{}, fmt.Errorf("error al cargar series: %v", err)
	}

	// Inicializar motor de reglas integrado
	manager.motorReglas = nuevoMotorReglasIntegrado(manager)
	manager.motorReglas.IniciarLimpiezaAutomatica()

	// Cargar reglas existentes desde PebbleDB
	err = manager.cargarReglasExistentes()
	if err != nil {
		return ManagerEdge{}, fmt.Errorf("error al cargar reglas: %v", err)
	}

	// Conectar a NATS (opcional - el nodo puede funcionar sin conexión)
	cliente, err := sw_cliente.Conectar(direccionNATS, puertoNATS)
	if err != nil {
		log.Printf("ADVERTENCIA: Nodo edge funcionando en modo autónomo sin NATS: %v", err)
		log.Printf("El nodo continuará operando localmente. Las funciones de cluster están deshabilitadas.")
		manager.cliente = nil

		go manager.intentarReconexionPeriodica()
	} else {
		manager.cliente = cliente

		if err := manager.sincronizarEstado(); err != nil {
			log.Printf("Error al sincronizar estado inicial: %v", err)
		}

		go manager.enviarHeartbeat()

		log.Printf("Nodo edge conectado al cluster vía NATS")
	}

	return *manager, nil
}

// generarNodeID genera un ID único para el nodo edge
func generarNodeID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("edge-%s-%s", hostname, shortUUID)
}

// ObtenerNodeID retorna el ID del nodo edge
func (me *ManagerEdge) ObtenerNodeID() string {
	return me.nodeID
}

// cargarSeriesExistentes carga todas las series desde PebbleDB al cache
func (me *ManagerEdge) cargarSeriesExistentes() error {
	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("series/"),
		UpperBound: []byte("series0"), // Rango que incluye todas las claves "series/*"
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if !strings.HasPrefix(key, "series/") {
			continue
		}

		// Extraer path de la clave
		seriesPath := strings.TrimPrefix(key, "series/")
		if seriesPath == "" {
			continue
		}

		// Deserializar configuración de serie
		var config Serie
		buffer := bytes.NewBuffer(iter.Value())
		decoder := gob.NewDecoder(buffer)
		if err := decoder.Decode(&config); err != nil {
			continue // Skip series con error de deserialización
		}

		// Retrocompatibilidad: si Path está vacío, usar NombreSerie
		if config.Path == "" {
			config.Path = config.NombreSerie
		}

		// Inicializar Tags si es nil
		if config.Tags == nil {
			config.Tags = make(map[string]string)
		}

		// Agregar al cache usando Path como clave
		me.cache.mu.Lock()
		me.cache.datos[seriesPath] = config
		me.cache.mu.Unlock()

		// Crear buffer y goroutine para cada serie
		serieBuffer := &SerieBuffer{
			datos:      make([]Medicion, config.TamañoBloque),
			serie:      config,
			indice:     0,
			done:       make(chan struct{}),
			datosCanal: make(chan Medicion, 100),
		}

		me.buffers.Store(seriesPath, serieBuffer)
		go me.manejarBuffer(serieBuffer)
	}

	return iter.Error()
}

// generarClaveSerie genera una clave PebbleDB para metadatos de serie
func generarClaveSerie(nombreSerie string) []byte {
	return []byte("series/" + nombreSerie)
}

// generarClaveSerieConPath genera una clave usando Path (nueva API)
func generarClaveSerieConPath(path string) []byte {
	return []byte("series/" + path)
}

// generarSeriesKey genera un identificador único para una serie basado en Path y Tags
func generarSeriesKey(path string, tags map[string]string) string {
	if tags == nil || len(tags) == 0 {
		return path
	}

	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	parts = append(parts, path)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, tags[k]))
	}
	return strings.Join(parts, ",")
}

// generarClaveDatos genera una clave PebbleDB incluyendo el tipo de datos
func generarClaveDatos(serieId int, tipoDatos TipoDatos, tiempoInicio, tiempoFin int64) []byte {
	key := fmt.Sprintf("data/%s/%010d/%020d_%020d", string(tipoDatos), serieId, tiempoInicio, tiempoFin)
	return []byte(key)
}

// serializarSerie serializa una configuración de Serie a bytes
func serializarSerie(serie Serie) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(serie)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Cerrar cierra la conexión a PebbleDB y todos los goroutines asociados
func (me *ManagerEdge) Cerrar() error {
	// Señalar a todos los goroutines que deben terminar
	close(me.done)

	// Cerrar todos los buffers individuales
	me.buffers.Range(func(key, value interface{}) bool {
		buffer := value.(*SerieBuffer)
		close(buffer.done)
		return true
	})

	// Desconectar cliente NATS
	if me.cliente != nil {
		me.cliente.Desconectar()
	}

	return me.db.Close()
}

// CrearSerie crea una nueva serie si no existe. Si ya existe, no hace nada.
func (me *ManagerEdge) CrearSerie(config Serie) error {
	// Generar clave única basada en Path o NombreSerie (retrocompatibilidad)
	seriesKey := config.Path
	if seriesKey == "" {
		seriesKey = config.NombreSerie
		config.Path = config.NombreSerie
	}

	// Inicializar Tags si es nil
	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}

	// Verificar si la serie ya existe en cache
	me.cache.mu.RLock()
	if _, exists := me.cache.datos[seriesKey]; exists {
		me.cache.mu.RUnlock()
		return nil
	}
	me.cache.mu.RUnlock()

	// Verificar si existe en PebbleDB
	key := generarClaveSerieConPath(seriesKey)
	_, closer, err := me.db.Get(key)
	if err == nil {
		closer.Close()
		return nil // Serie ya existe
	}
	if err != pebble.ErrNotFound {
		return fmt.Errorf("error al verificar serie: %v", err)
	}

	// Generar nuevo ID para la serie
	me.mu.Lock()
	me.counter++
	config.SerieId = me.counter

	// Actualizar contador en PebbleDB
	counterBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(counterBytes, uint32(me.counter))
	err = me.db.Set([]byte("meta/counter"), counterBytes, pebble.Sync)
	me.mu.Unlock()

	if err != nil {
		return fmt.Errorf("error al actualizar contador: %v", err)
	}

	// Serializar y guardar configuración de serie
	serieBytes, err := serializarSerie(config)
	if err != nil {
		return fmt.Errorf("error al serializar serie: %v", err)
	}

	err = me.db.Set(key, serieBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al guardar serie: %v", err)
	}

	// Actualizar cache usando Path como clave
	me.cache.mu.Lock()
	me.cache.datos[seriesKey] = config
	me.cache.mu.Unlock()

	// Crear buffer y goroutine para la nueva serie
	buffer := &SerieBuffer{
		datos:      make([]Medicion, config.TamañoBloque),
		serie:      config,
		indice:     0,
		done:       make(chan struct{}),
		datosCanal: make(chan Medicion, 100),
	}

	me.buffers.Store(seriesKey, buffer)
	go me.manejarBuffer(buffer)

	// Informar nueva serie al despachador
	if me.cliente != nil {
		if err := me.informarNuevaSerie(seriesKey, config.SerieId); err != nil {
			log.Printf("Error al informar nueva serie: %v", err)
		}
	}

	return nil
}

func (me *ManagerEdge) ObtenerSeries(nombreSerie string) (Serie, error) {
	// Lectura sin bloqueo para casos comunes
	me.cache.mu.RLock()
	if meta, exists := me.cache.datos[nombreSerie]; exists {
		me.cache.mu.RUnlock()
		return meta, nil
	}
	me.cache.mu.RUnlock()

	// Cache miss - error
	return Serie{}, fmt.Errorf("Serie no encontrada")
}

func (me *ManagerEdge) ListarSeries() ([]string, error) {
	me.cache.mu.RLock()
	defer me.cache.mu.RUnlock()

	nombres := make([]string, 0, len(me.cache.datos))
	for nombre := range me.cache.datos {
		nombres = append(nombres, nombre)
	}
	return nombres, nil
}

func (me *ManagerEdge) manejarBuffer(buffer *SerieBuffer) {
	for {
		select {
		case <-buffer.done:
			return
		case <-me.done:
			return
		case medicion := <-buffer.datosCanal:
			buffer.mu.Lock()
			// Agregar la medición al buffer
			buffer.datos[buffer.indice] = medicion
			buffer.indice++

			// Verificar si el buffer está lleno
			if buffer.indice >= buffer.serie.TamañoBloque {
				// Comprimir y almacenar el buffer
				me.comprimirYAlmacenar(buffer)
				// Limpiar el buffer
				buffer.indice = 0
				// Limpiar los datos (opcional, se sobreescribirán)
				for i := range buffer.datos {
					buffer.datos[i] = Medicion{}
				}
			}
			buffer.mu.Unlock()
		}
	}
}

func (me *ManagerEdge) Insertar(nombreSerie string, tiempo int64, dato interface{}) error {
	// Obtener el buffer para la serie
	bufferInterface, ok := me.buffers.Load(nombreSerie)
	if !ok {
		return fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

	buffer := bufferInterface.(*SerieBuffer)

	// Inferencia automática de tipo si no está definido
	if buffer.serie.TipoDatos == TipoMixto || buffer.serie.TipoDatos == "" {
		tipoInferido := inferirTipo(dato)
		buffer.serie.TipoDatos = tipoInferido

		// Actualizar la serie en cache y base de datos
		me.cache.mu.Lock()
		me.cache.datos[nombreSerie] = buffer.serie
		me.cache.mu.Unlock()

		// Actualizar en PebbleDB
		key := generarClaveSerie(nombreSerie)
		serieBytes, err := serializarSerie(buffer.serie)
		if err == nil {
			me.db.Set(key, serieBytes, pebble.Sync)
		}
	} else {
		// Validar compatibilidad de tipo
		if !esCompatibleConTipo(dato, buffer.serie.TipoDatos) {
			return fmt.Errorf("tipo de dato incompatible: esperado %s, recibido %T",
				buffer.serie.TipoDatos, dato)
		}
	}

	// Crear la medición con tiempo y valor
	medicion := Medicion{
		Tiempo: tiempo,
		Valor:  dato,
	}

	// Enviar la medición al canal del buffer
	select {
	case buffer.datosCanal <- medicion:
		return nil
	default:
		return fmt.Errorf("buffer del canal lleno para la serie: %s", nombreSerie)
	}
}

// inferirTipo determina el tipo de datos basado en el valor proporcionado
func inferirTipo(valor interface{}) TipoDatos {
	switch valor.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return TipoNumerico
	case string:
		return TipoCategorico
	default:
		return TipoMixto
	}
}

// esCompatibleConTipo verifica si un valor es compatible con el tipo de serie
func esCompatibleConTipo(valor interface{}, tipoDatos TipoDatos) bool {
	tipoValor := inferirTipo(valor)

	switch tipoDatos {
	case TipoNumerico:
		return tipoValor == TipoNumerico
	case TipoCategorico:
		return tipoValor == TipoCategorico
	case TipoMixto:
		return tipoValor == TipoNumerico || tipoValor == TipoCategorico
	case "": // Series sin tipo definido (retrocompatibilidad)
		return true // Acepta cualquier tipo si no está definido
	default:
		return tipoDatos == TipoMixto // Por defecto asumir mixto
	}
}

// deberiaSkipearBloque determina si un bloque debe ser omitido basado en su rango temporal
func (me *ManagerEdge) deberiaSkipearBloque(key string, tiempoInicio, tiempoFin int64) bool {
	parts := strings.Split(key, "/")

	// Formato: data/TIPO/XXXXXXXXXX/TTTTTTTTTTTTTTTTTTTT_TTTTTTTTTTTTTTTTTTTT
	if len(parts) != 4 {
		return false // Formato desconocido, no skip
	}

	timeRange := parts[3]
	timeParts := strings.Split(timeRange, "_")
	if len(timeParts) != 2 {
		return false // Formato sin rango, no skip
	}

	bloqueInicio, err1 := strconv.ParseInt(timeParts[0], 10, 64)
	bloqueFin, err2 := strconv.ParseInt(timeParts[1], 10, 64)

	if err1 != nil || err2 != nil {
		return false // Error parsing, no skip por seguridad
	}

	// Skip si no hay superposición temporal:
	// El bloque termina antes que inicie nuestro rango, O
	// El bloque inicia después que termine nuestro rango
	if bloqueFin < tiempoInicio || bloqueInicio > tiempoFin {
		return true
	}

	return false
}

// ConsultarRango consulta mediciones de una serie dentro de un rango de tiempo
// y descomprime los datos antes de retornarlos
func (me *ManagerEdge) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]Medicion, error) {
	// Convertir los tiempos a Unix timestamp (en nanosegundos)
	tiempoInicioUnix := tiempoInicio.UnixNano()
	tiempoFinUnix := tiempoFin.UnixNano()
	// Obtener configuración de la serie desde cache
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return nil, fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

	var resultados []Medicion

	// Crear rangos de búsqueda para iterar sobre los datos de la serie
	keyPrefix := fmt.Sprintf("data/%s/%010d/", string(serie.TipoDatos), serie.SerieId)
	lowerBound := []byte(keyPrefix)
	upperBound := []byte(keyPrefix + "~") // '~' es mayor que todos los números

	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Iterar sobre todos los bloques de la serie
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())

		// Extraer timestamps del rango del bloque desde la clave para skip temprano
		// Formato: data/TIPO/XXXXXXXXXX/TTTTTTTTTTTTTTTTTTTT_TTTTTTTTTTTTTTTTTTTT
		if skipBloque := me.deberiaSkipearBloque(key, tiempoInicioUnix, tiempoFinUnix); skipBloque {
			continue // Skip este bloque sin descomprimirlo
		}

		datosComprimidos := make([]byte, len(iter.Value()))
		copy(datosComprimidos, iter.Value())

		// Descomprimir el bloque
		mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
		if err != nil {
			fmt.Printf("Error al descomprimir bloque: %v\n", err)
			continue
		}

		// Filtrar mediciones que están dentro del rango solicitado
		for _, medicion := range mediciones {
			if medicion.Tiempo >= tiempoInicioUnix && medicion.Tiempo <= tiempoFinUnix {
				resultados = append(resultados, medicion)
			}
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("error al iterar sobre datos: %v", err)
	}

	// Obtener datos del buffer en memoria si existe
	if bufferInterface, ok := me.buffers.Load(nombreSerie); ok {
		buffer := bufferInterface.(*SerieBuffer)
		buffer.mu.Lock()

		// Revisar datos del buffer que están dentro del rango
		for i := 0; i < buffer.indice; i++ {
			medicion := buffer.datos[i]
			if medicion.Tiempo >= tiempoInicioUnix && medicion.Tiempo <= tiempoFinUnix {
				resultados = append(resultados, medicion)
			}
		}

		buffer.mu.Unlock()
	}

	return resultados, nil
}

// descomprimirBloque descomprime un bloque de datos usando el proceso inverso
// al de comprimirYAlmacenar
func (me *ManagerEdge) descomprimirBloque(datosComprimidos []byte, serie Serie) ([]Medicion, error) {
	// NIVEL 2: Descompresión de bloque
	compresorBloque := me.obtenerCompressorBloque(serie.CompresionBloque)
	bloqueDescomprimido, err := compresorBloque.Descomprimir(datosComprimidos)
	if err != nil {
		return nil, fmt.Errorf("error en descompresión de bloque: %v", err)
	}

	// NIVEL 1: Separar datos de tiempos y valores
	tiemposComprimidos, valoresComprimidos, err := separarDatos(bloqueDescomprimido)
	if err != nil {
		return nil, fmt.Errorf("error al separar datos: %v", err)
	}

	// Descomprimir tiempos usando DeltaDelta
	tiempos, err := me.descompresionDeltaDeltaTiempo(tiemposComprimidos)
	if err != nil {
		return nil, fmt.Errorf("error al descomprimir tiempos: %v", err)
	}

	// Descomprimir valores usando el compresor configurado para la serie
	compresorValor := me.obtenerCompressorValor(serie.CompresionBytes)
	valores, err := compresorValor.Descomprimir(valoresComprimidos)
	if err != nil {
		return nil, fmt.Errorf("error al descomprimir valores: %v", err)
	}

	// Verificar que tengamos el mismo número de tiempos y valores
	if len(tiempos) != len(valores) {
		return nil, fmt.Errorf("número de tiempos (%d) y valores (%d) no coinciden", len(tiempos), len(valores))
	}

	// Reconstruir las mediciones
	mediciones := make([]Medicion, len(tiempos))
	for i := 0; i < len(tiempos); i++ {
		mediciones[i] = Medicion{
			Tiempo: tiempos[i],
			Valor:  valores[i],
		}
	}

	return mediciones, nil
}

func (me *ManagerEdge) comprimirYAlmacenar(buffer *SerieBuffer) {
	// Obtener mediciones válidas del buffer
	mediciones := buffer.datos[:buffer.indice]
	if len(mediciones) == 0 {
		return
	}

	// NIVEL 1: Compresión específica
	// Tiempo: SIEMPRE usar DeltaDelta
	tiemposComprimidos := me.compresionDeltaDeltaTiempo(mediciones)

	// Valores: usar compresión configurada en la serie
	valores := extraerValores(mediciones)
	compresorValor := me.obtenerCompressorValor(buffer.serie.CompresionBytes)
	valoresComprimidos := compresorValor.Comprimir(valores)

	// Combinar datos del nivel 1
	bloqueNivel1 := combinarDatos(tiemposComprimidos, valoresComprimidos)

	// NIVEL 2: Compresión de bloque
	compresorBloque := me.obtenerCompressorBloque(buffer.serie.CompresionBloque)
	bloqueFinal, _ := compresorBloque.Comprimir(bloqueNivel1)

	// Calcular timestamps de inicio y fin
	tiempoInicio := mediciones[0].Tiempo
	tiempoFinal := mediciones[len(mediciones)-1].Tiempo

	fmt.Println("Almacenando bloque para serie:", buffer.serie.NombreSerie,
		"Tiempo inicio:", tiempoInicio,
		"Tiempo final:", tiempoFinal,
		"Mediciones:", len(mediciones),
		"Tamaño comprimido:", len(bloqueFinal))

	// Escribir directamente a PebbleDB (sin serialización)
	key := generarClaveDatos(buffer.serie.SerieId, buffer.serie.TipoDatos, tiempoInicio, tiempoFinal)
	err := me.db.Set(key, bloqueFinal, pebble.Sync)
	if err != nil {
		fmt.Printf("Error al escribir datos para serie %s: %v\n", buffer.serie.NombreSerie, err)
	}
}

// ConsultarUltimoPunto obtiene la última medición registrada para una serie
func (me *ManagerEdge) ConsultarUltimoPunto(nombreSerie string) (Medicion, error) {
	// Primero revisar el buffer en memoria
	if bufferInterface, ok := me.buffers.Load(nombreSerie); ok {
		buffer := bufferInterface.(*SerieBuffer)
		buffer.mu.Lock()
		defer buffer.mu.Unlock()

		if buffer.indice > 0 {
			// Encontrar la medición más reciente en el buffer
			ultimaMedicion := buffer.datos[0]
			for i := 1; i < buffer.indice; i++ {
				if buffer.datos[i].Tiempo > ultimaMedicion.Tiempo {
					ultimaMedicion = buffer.datos[i]
				}
			}
			return ultimaMedicion, nil
		}
	}

	// Si no hay datos en buffer, consultar la base de datos
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return Medicion{}, fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

	// Buscar el último bloque para esta serie
	keyPrefix := fmt.Sprintf("data/%s/%010d/", string(serie.TipoDatos), serie.SerieId)
	lowerBound := []byte(keyPrefix)
	upperBound := []byte(keyPrefix + "~")

	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return Medicion{}, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Ir al último elemento
	if !iter.Last() {
		return Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", nombreSerie)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return Medicion{}, fmt.Errorf("error al descomprimir último bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return Medicion{}, fmt.Errorf("bloque vacío para serie: %s", nombreSerie)
	}

	// Encontrar la medición más reciente en el bloque
	ultimaMedicion := mediciones[0]
	for _, m := range mediciones[1:] {
		if m.Tiempo > ultimaMedicion.Tiempo {
			ultimaMedicion = m
		}
	}

	return ultimaMedicion, nil
}

// ConsultarPrimerPunto obtiene la primera medición registrada para una serie
func (me *ManagerEdge) ConsultarPrimerPunto(nombreSerie string) (Medicion, error) {
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return Medicion{}, fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

	// Buscar el primer bloque para esta serie
	keyPrefix := fmt.Sprintf("data/%s/%010d/", string(serie.TipoDatos), serie.SerieId)
	lowerBound := []byte(keyPrefix)
	upperBound := []byte(keyPrefix + "~")

	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return Medicion{}, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Ir al primer elemento
	if !iter.First() {
		return Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", nombreSerie)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return Medicion{}, fmt.Errorf("error al descomprimir primer bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return Medicion{}, fmt.Errorf("bloque vacío para serie: %s", nombreSerie)
	}

	// Encontrar la medición más antigua en el bloque
	primeraMedicion := mediciones[0]
	for _, m := range mediciones[1:] {
		if m.Tiempo < primeraMedicion.Tiempo {
			primeraMedicion = m
		}
	}

	return primeraMedicion, nil
}

// generarClaveRegla genera una clave PebbleDB para metadatos de regla
func generarClaveRegla(id string) []byte {
	return []byte("reglas/" + id)
}

// serializarRegla serializa una Regla a bytes
func serializarRegla(regla *Regla) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(regla)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// deserializarRegla deserializa bytes a una Regla
func deserializarRegla(data []byte) (*Regla, error) {
	var regla Regla
	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)
	err := decoder.Decode(&regla)
	if err != nil {
		return nil, err
	}
	return &regla, nil
}

// cargarReglasExistentes carga todas las reglas desde PebbleDB al motor
func (me *ManagerEdge) cargarReglasExistentes() error {
	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("reglas/"),
		UpperBound: []byte("reglas0"), // Rango que incluye todas las claves "reglas/*"
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if !strings.HasPrefix(key, "reglas/") {
			continue
		}

		// Deserializar regla
		regla, err := deserializarRegla(iter.Value())
		if err != nil {
			continue // Skip reglas con error de deserialización
		}

		// Agregar al motor (solo en memoria, ya está en DB)
		me.motorReglas.mu.Lock()
		me.motorReglas.reglas[regla.ID] = regla
		me.motorReglas.mu.Unlock()
	}

	return iter.Error()
}

// Métodos proxy para el motor de reglas
func (me *ManagerEdge) AgregarRegla(regla *Regla) error {
	if regla == nil {
		return fmt.Errorf("regla no puede ser nil")
	}

	if err := me.motorReglas.validarRegla(regla); err != nil {
		return fmt.Errorf("regla inválida: %v", err)
	}

	// Guardar en base de datos
	key := generarClaveRegla(regla.ID)
	reglaBytes, err := serializarRegla(regla)
	if err != nil {
		return fmt.Errorf("error al serializar regla: %v", err)
	}

	err = me.db.Set(key, reglaBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al guardar regla: %v", err)
	}

	// Agregar al cache en memoria
	me.motorReglas.mu.Lock()
	defer me.motorReglas.mu.Unlock()

	regla.Activa = true
	me.motorReglas.reglas[regla.ID] = regla

	log.Printf("Regla '%s' agregada exitosamente", regla.ID)
	return nil
}

func (me *ManagerEdge) EliminarRegla(id string) error {
	me.motorReglas.mu.Lock()
	defer me.motorReglas.mu.Unlock()

	if _, exists := me.motorReglas.reglas[id]; !exists {
		return fmt.Errorf("regla '%s' no encontrada", id)
	}

	// Eliminar de base de datos
	key := generarClaveRegla(id)
	err := me.db.Delete(key, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al eliminar regla de DB: %v", err)
	}

	// Eliminar del cache
	delete(me.motorReglas.reglas, id)

	log.Printf("Regla '%s' eliminada", id)
	return nil
}

func (me *ManagerEdge) ActualizarRegla(regla *Regla) error {
	if regla == nil {
		return fmt.Errorf("regla no puede ser nil")
	}

	if err := me.motorReglas.validarRegla(regla); err != nil {
		return fmt.Errorf("regla inválida: %v", err)
	}

	me.motorReglas.mu.Lock()
	defer me.motorReglas.mu.Unlock()

	if _, exists := me.motorReglas.reglas[regla.ID]; !exists {
		return fmt.Errorf("regla '%s' no encontrada", regla.ID)
	}

	// Actualizar en base de datos
	key := generarClaveRegla(regla.ID)
	reglaBytes, err := serializarRegla(regla)
	if err != nil {
		return fmt.Errorf("error al serializar regla: %v", err)
	}

	err = me.db.Set(key, reglaBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al actualizar regla en DB: %v", err)
	}

	// Actualizar en cache
	me.motorReglas.reglas[regla.ID] = regla
	log.Printf("Regla '%s' actualizada", regla.ID)
	return nil
}

func (me *ManagerEdge) ListarReglas() map[string]*Regla {
	me.motorReglas.mu.RLock()
	defer me.motorReglas.mu.RUnlock()

	copia := make(map[string]*Regla)
	for id, regla := range me.motorReglas.reglas {
		copia[id] = regla
	}
	return copia
}

func (me *ManagerEdge) ObtenerRegla(id string) (*Regla, error) {
	me.motorReglas.mu.RLock()
	defer me.motorReglas.mu.RUnlock()

	regla, exists := me.motorReglas.reglas[id]
	if !exists {
		return nil, fmt.Errorf("regla '%s' no encontrada", id)
	}

	return regla, nil
}

func (me *ManagerEdge) ProcesarDatoRegla(serie string, valor float64, timestamp time.Time) error {
	return me.motorReglas.ProcesarDato(serie, valor, timestamp)
}

func (me *ManagerEdge) RegistrarEjecutor(tipoAccion string, ejecutor EjecutorAccion) error {
	return me.motorReglas.RegistrarEjecutor(tipoAccion, ejecutor)
}

func (me *ManagerEdge) HabilitarMotorReglas(habilitado bool) {
	me.motorReglas.Habilitar(habilitado)
}

func (me *ManagerEdge) informarSuscripcion() error {
	if me.cliente == nil {
		return nil
	}

	me.cache.mu.RLock()
	series := make(map[string]string)
	for path, serie := range me.cache.datos {
		series[path] = fmt.Sprintf("%d", serie.SerieId)
	}
	me.cache.mu.RUnlock()

	suscripcion := SuscripcionNodo{
		ID:     me.nodeID,
		Series: series,
	}

	payload, err := json.Marshal(suscripcion)
	if err != nil {
		return fmt.Errorf("error al serializar suscripción: %v", err)
	}

	me.cliente.Publicar("despachador.suscripcion", payload)
	log.Printf("Suscripción informada: nodo %s con %d series", me.nodeID, len(series))
	return nil
}

func (me *ManagerEdge) informarNuevaSerie(path string, serieID int) error {
	if me.cliente == nil {
		return nil
	}

	nuevaSerie := NuevaSerie{
		NodeID:  me.nodeID,
		Path:    path,
		SerieID: serieID,
	}

	payload, err := json.Marshal(nuevaSerie)
	if err != nil {
		return fmt.Errorf("error al serializar nueva serie: %v", err)
	}

	me.cliente.Publicar("despachador.nueva_serie", payload)
	log.Printf("Nueva serie informada: %s (ID: %d) en nodo %s", path, serieID, me.nodeID)
	return nil
}

func (me *ManagerEdge) enviarHeartbeat() {
	if me.cliente == nil {
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-me.done:
			return
		case <-ticker.C:
			if me.cliente == nil {
				return
			}

			heartbeat := Heartbeat{
				NodeID:    me.nodeID,
				Timestamp: time.Now(),
				Activo:    true,
			}

			payload, err := json.Marshal(heartbeat)
			if err != nil {
				log.Printf("Error al serializar heartbeat: %v", err)
				continue
			}

			me.cliente.Publicar("despachador.heartbeat", payload)
			log.Printf("Heartbeat enviado desde nodo %s", me.nodeID)
		}
	}
}

func (me *ManagerEdge) intentarReconexionPeriodica() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-me.done:
			return
		case <-ticker.C:
			me.muConexion.Lock()
			if me.cliente != nil {
				me.muConexion.Unlock()
				return
			}

			if me.reconectando {
				me.muConexion.Unlock()
				continue
			}

			me.reconectando = true
			me.muConexion.Unlock()

			log.Printf("Intentando reconectar a NATS...")
			cliente, err := sw_cliente.Conectar(me.direccionNATS, me.puertoNATS)

			me.muConexion.Lock()
			if err != nil {
				log.Printf("Fallo al reconectar a NATS: %v", err)
				me.reconectando = false
				me.muConexion.Unlock()
				continue
			}

			me.cliente = cliente
			me.reconectando = false
			me.muConexion.Unlock()

			log.Printf("Reconexión a NATS exitosa")

			if err := me.sincronizarEstado(); err != nil {
				log.Printf("Error al sincronizar estado después de reconexión: %v", err)
			}

			go me.enviarHeartbeat()
			return
		}
	}
}

func (me *ManagerEdge) sincronizarEstado() error {
	me.muSincronizando.Lock()
	defer me.muSincronizando.Unlock()

	if me.cliente == nil {
		return fmt.Errorf("cliente NATS no disponible")
	}

	if err := me.informarSuscripcion(); err != nil {
		return fmt.Errorf("error al informar suscripción: %v", err)
	}

	me.ultimaSincro = time.Now()
	log.Printf("Estado sincronizado exitosamente para nodo %s", me.nodeID)
	return nil
}

func (me *ManagerEdge) manejarDesconexion() {
	me.muConexion.Lock()
	defer me.muConexion.Unlock()

	if me.cliente != nil {
		me.cliente.Desconectar()
		me.cliente = nil
		log.Printf("Desconectado de NATS, cambiando a modo autónomo")
	}

	go me.intentarReconexionPeriodica()
}

func (me *ManagerEdge) EstadoConexion() string {
	me.muConexion.Lock()
	defer me.muConexion.Unlock()

	if me.cliente != nil {
		return "conectado"
	}
	if me.reconectando {
		return "reconectando"
	}
	return "desconectado"
}

func (me *ManagerEdge) ObtenerUltimaSincronizacion() time.Time {
	me.muSincronizando.Lock()
	defer me.muSincronizando.Unlock()
	return me.ultimaSincro
}

// ListarSeriesPorPath retorna todas las series que coincidan con un patrón de path
// Soporta wildcards: "dispositivo_001/*" o "*/temperatura"
func (me *ManagerEdge) ListarSeriesPorPath(pathPattern string) ([]Serie, error) {
	me.cache.mu.RLock()
	defer me.cache.mu.RUnlock()

	var series []Serie
	for _, serie := range me.cache.datos {
		if matchPath(serie.Path, pathPattern) {
			series = append(series, serie)
		}
	}
	return series, nil
}

// ListarSeriesPorTags retorna todas las series que tengan todos los tags especificados
func (me *ManagerEdge) ListarSeriesPorTags(tags map[string]string) ([]Serie, error) {
	me.cache.mu.RLock()
	defer me.cache.mu.RUnlock()

	var series []Serie
	for _, serie := range me.cache.datos {
		if matchTags(serie.Tags, tags) {
			series = append(series, serie)
		}
	}
	return series, nil
}

// ListarSeriesPorDispositivo retorna todas las series de un dispositivo específico
// Asume que el path es "dispositivo_XXX/metrica"
func (me *ManagerEdge) ListarSeriesPorDispositivo(dispositivoID string) ([]Serie, error) {
	pathPattern := dispositivoID + "/*"
	return me.ListarSeriesPorPath(pathPattern)
}

// ConsultarRangoPorPath consulta mediciones usando path (con soporte para NombreSerie legacy)
func (me *ManagerEdge) ConsultarRangoPorPath(path string, tiempoInicio, tiempoFin time.Time) ([]Medicion, error) {
	return me.ConsultarRango(path, tiempoInicio, tiempoFin)
}

// matchPath verifica si un path coincide con un patrón (soporta wildcard *)
func matchPath(path, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if !strings.Contains(pattern, "*") {
		return path == pattern
	}

	parts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(parts) != len(pathParts) {
		return false
	}

	for i, part := range parts {
		if part == "*" {
			continue
		}
		if part != pathParts[i] {
			return false
		}
	}

	return true
}

// matchTags verifica si una serie tiene todos los tags especificados
func matchTags(serieTags, filterTags map[string]string) bool {
	if len(filterTags) == 0 {
		return true
	}

	for key, value := range filterTags {
		if serieValue, exists := serieTags[key]; !exists || serieValue != value {
			return false
		}
	}

	return true
}
