package edge

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/google/uuid"
	"github.com/apple/foundationdb/bindings/go/src/fdb"

	"github.com/cbiale/sensorwave/compresor"
	"github.com/cbiale/sensorwave/tipos"
)

type ManagerEdge struct {
	db          *pebble.DB
	fdbDatabase *fdb.Database
	cache       *Cache
	buffers     sync.Map
	done        chan struct{}
	counter     int
	mu          sync.RWMutex
	MotorReglas *MotorReglas
	Cluster     *ClusterManager
	nodeID      string
}

type Cache struct {
	datos map[string]Serie // Mapa para almacenar las configuraciones de series
	mu    sync.RWMutex     // Mutex para proteger el acceso concurrente
}

type Serie struct {
	SerieId          int                  		 // ID de la serie en la base de datos
	Path             string                		 // Path jerárquico: "dispositivo_001/temperatura"
	Tags             map[string]string     		 // Tags: {"ubicacion": "sala1", "tipo": "DHT22"}
	TipoDatos        tipos.TipoDatos			 // Tipo de datos almacenados
	CompresionBloque tipos.TipoCompresionBloque  // Compresión nivel bloque
	CompresionBytes  tipos.TipoCompresionValores // Compresión nivel valores
	TamañoBloque     int                   		 // Tamaño del bloque
}

type SerieBuffer struct {
	datos      []tipos.Medicion     // Arreglo con TamañoBloque elementos
	serie      Serie         		// Configuración de la serie
	indice     int           		// Índice actual en el buffer
	mu         sync.Mutex   		// Mutex para proteger el buffer
	done       chan struct{} 		// Canal para señalar cierre del hilo
	datosCanal chan tipos.Medicion  // Canal para recibir nuevos datos
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

type SolicitudConsulta struct {
	Serie        string    `json:"serie"`
	TiempoInicio time.Time `json:"tiempo_inicio"`
	TiempoFin    time.Time `json:"tiempo_fin"`
}

type RespuestaConsulta struct {
	Mediciones []tipos.Medicion `json:"mediciones"`
	Error      string     		`json:"error,omitempty"`
}

// Crear inicializa el ManagerEdge con PebbleDB, cache y FoundationDB
func Crear(nombre string, direccionNATS string, puertoNATS string, nombreFDB string) (*ManagerEdge, error) {
	db, err := pebble.Open(nombre, &pebble.Options{})
	if err != nil {
		return &ManagerEdge{}, err
	}

	manager := &ManagerEdge{
		db: db,
		cache: &Cache{
			datos: make(map[string]Serie),
		},
		done:    make(chan struct{}),
		counter: 0,
	}

	// Cargar o generar nodeID
	nodeIDBytes, closer, err := db.Get([]byte("meta/node_id"))
	if err == pebble.ErrNotFound {
		manager.nodeID = generarNodeID()
		err = db.Set([]byte("meta/node_id"), []byte(manager.nodeID), pebble.Sync)
		if err != nil {
			return &ManagerEdge{}, fmt.Errorf("error al guardar node_id: %v", err)
		}
		log.Printf("Nuevo nodeID generado: %s", manager.nodeID)
	} else if err != nil {
		return &ManagerEdge{}, fmt.Errorf("error al leer node_id: %v", err)
	} else {
		manager.nodeID = string(nodeIDBytes)
		closer.Close()
		log.Printf("NodeID cargado desde DB: %s", manager.nodeID)
	}

	// Cargar contador de series desde PebbleDB
	counterBytes, closer, err := db.Get([]byte("meta/counter"))
	if err != nil && err != pebble.ErrNotFound {
		return &ManagerEdge{}, fmt.Errorf("error al leer contador: %v", err)
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
		return &ManagerEdge{}, fmt.Errorf("error al cargar series: %v", err)
	}

	// Conectar a FoundationDB si se proporciona coordinador
	if nombreFDB != "" {
		// Inicializar FoundationDB
		fdb.MustAPIVersion(730)

		db, err := fdb.OpenDatabase(nombreFDB)
    	if err != nil {
			manager.fdbDatabase = nil
			log.Printf("Modo local: error al realizar conexión a FoundationDB")
    	}

		manager.fdbDatabase = &db
		log.Printf("Conectado a FoundationDB en %s", nombreFDB)
	} else {
		log.Printf("Modo local: sin conexión a FoundationDB")
	}

	manager.MotorReglas = nuevoMotorReglasIntegrado(manager, db)
	manager.MotorReglas.IniciarLimpiezaAutomatica()

	err = manager.MotorReglas.cargarReglasExistentes()
	if err != nil {
		return &ManagerEdge{}, fmt.Errorf("error al cargar reglas: %v", err)
	}

	manager.Cluster = nuevoClusterManager(manager, manager.nodeID, direccionNATS, puertoNATS, manager.done)
	manager.Cluster.conectar()

	return manager, nil
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
			datos:      make([]tipos.Medicion, config.TamañoBloque),
			serie:      config,
			indice:     0,
			done:       make(chan struct{}),
			datosCanal: make(chan tipos.Medicion, 100),
		}

		me.buffers.Store(seriesPath, serieBuffer)
		go me.manejarBuffer(serieBuffer)
	}

	return iter.Error()
}

// generarClaveSerieConPath genera una clave usando Path (nueva API)
func generarClaveSerieConPath(path string) []byte {
	return []byte("series/" + path)
}

// generarClaveDatos genera una clave PebbleDB incluyendo el tipo de datos
func generarClaveDatos(serieId int, tipoDatos tipos.TipoDatos, tiempoInicio, tiempoFin int64) []byte {
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

// Cerrar cierra la conexión a PebbleDB, FoundationDB y todos los goroutines asociados
func (me *ManagerEdge) Cerrar() error {
	// Señalar a todos los goroutines que deben terminar
	close(me.done)

	// Cerrar todos los buffers individuales
	me.buffers.Range(func(key, value interface{}) bool {
		buffer := value.(*SerieBuffer)
		close(buffer.done)
		return true
	})

	if me.Cluster != nil {
		me.Cluster.cerrar()
	}

	// Cerrar conexión a FoundationDB si existe
	if me.fdbDatabase != nil {
		// TODO: Implementar cierre de FoundationDB cuando esté disponible
		log.Printf("Cerrando conexión a FoundationDB")
	}

	return me.db.Close()
}

// CrearSerie crea una nueva serie si no existe. Si ya existe, no hace nada.
func (me *ManagerEdge) CrearSerie(config Serie) error {
	// Generar clave única basada en Path
	seriesKey := config.Path

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
		datos:      make([]tipos.Medicion, config.TamañoBloque),
		serie:      config,
		indice:     0,
		done:       make(chan struct{}),
		datosCanal: make(chan tipos.Medicion, 100),
	}

	me.buffers.Store(seriesKey, buffer)
	go me.manejarBuffer(buffer)

	if me.Cluster != nil {
		if err := me.Cluster.InformarNuevaSerie(seriesKey, config.SerieId); err != nil {
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
					buffer.datos[i] = tipos.Medicion{}
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
	if buffer.serie.TipoDatos == tipos.TipoMixto || buffer.serie.TipoDatos == "" {
		tipoInferido := inferirTipo(dato)
		buffer.serie.TipoDatos = tipoInferido

		// Actualizar la serie en cache y base de datos
		me.cache.mu.Lock()
		me.cache.datos[nombreSerie] = buffer.serie
		me.cache.mu.Unlock()

		// Actualizar en PebbleDB
		key := generarClaveSerieConPath(nombreSerie)
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
	medicion := tipos.Medicion{
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
func inferirTipo(valor interface{}) tipos.TipoDatos {
	switch valor.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return tipos.TipoNumerico
	case string:
		return tipos.TipoCategorico
	default:
		return tipos.TipoMixto
	}
}

// esCompatibleConTipo verifica si un valor es compatible con el tipo de serie
func esCompatibleConTipo(valor interface{}, tipoDatos tipos.TipoDatos) bool {
	tipoValor := inferirTipo(valor)

	switch tipoDatos {
	case tipos.TipoNumerico:
		return tipoValor == tipos.TipoNumerico
	case tipos.TipoCategorico:
		return tipoValor == tipos.TipoCategorico
	case tipos.TipoMixto:
		return tipoValor == tipos.TipoNumerico || tipoValor == tipos.TipoCategorico
	case "": // Series sin tipo definido (retrocompatibilidad)
		return true // Acepta cualquier tipo si no está definido
	default:
		return tipoDatos == tipos.TipoMixto // Por defecto asumir mixto
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
func (me *ManagerEdge) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]tipos.Medicion, error) {
	// Convertir los tiempos a Unix timestamp (en nanosegundos)
	tiempoInicioUnix := tiempoInicio.UnixNano()
	tiempoFinUnix := tiempoFin.UnixNano()
	// Obtener configuración de la serie desde cache
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return nil, fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

	var resultados []tipos.Medicion

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
func (me *ManagerEdge) descomprimirBloque(datosComprimidos []byte, serie Serie) ([]tipos.Medicion, error) {
	// NIVEL 2: Descompresión de bloque
	compresorBloque := compresor.ObtenerCompressorBloque(serie.CompresionBloque)
	bloqueDescomprimido, err := compresorBloque.Descomprimir(datosComprimidos)
	if err != nil {
		return nil, fmt.Errorf("error en descompresión de bloque: %v", err)
	}

	// NIVEL 1: Separar datos de tiempos y valores
	tiemposComprimidos, valoresComprimidos, err := compresor.SepararDatos(bloqueDescomprimido)
	if err != nil {
		return nil, fmt.Errorf("error al separar datos: %v", err)
	}

	// Descomprimir tiempos usando DeltaDelta
	tiempos, err := compresor.DescompresionDeltaDeltaTiempo(tiemposComprimidos)
	if err != nil {
		return nil, fmt.Errorf("error al descomprimir tiempos: %v", err)
	}

	// Descomprimir valores usando el compresor configurado para la serie
	compresorValor := compresor.ObtenerCompressorValor(serie.CompresionBytes)
	valores, err := compresorValor.Descomprimir(valoresComprimidos)
	if err != nil {
		return nil, fmt.Errorf("error al descomprimir valores: %v", err)
	}

	// Verificar que tengamos el mismo número de tiempos y valores
	if len(tiempos) != len(valores) {
		return nil, fmt.Errorf("número de tiempos (%d) y valores (%d) no coinciden", len(tiempos), len(valores))
	}

	// Reconstruir las mediciones
	mediciones := make([]tipos.Medicion, len(tiempos))
	for i := 0; i < len(tiempos); i++ {
		mediciones[i] = tipos.Medicion{
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
	tiemposComprimidos := compresor.CompresionDeltaDeltaTiempo(mediciones)

	// Valores: usar compresión configurada en la serie
	valores := compresor.ExtraerValores(mediciones)
	compresorValor := compresor.ObtenerCompressorValor(buffer.serie.CompresionBytes)
	valoresComprimidos := compresorValor.Comprimir(valores)

	// Combinar datos del nivel 1
	bloqueNivel1 := compresor.CombinarDatos(tiemposComprimidos, valoresComprimidos)

	// NIVEL 2: Compresión de bloque
	compresorBloque := compresor.ObtenerCompressorBloque(buffer.serie.CompresionBloque)
	bloqueFinal, _ := compresorBloque.Comprimir(bloqueNivel1)

	// Calcular timestamps de inicio y fin
	tiempoInicio := mediciones[0].Tiempo
	tiempoFinal := mediciones[len(mediciones)-1].Tiempo

	fmt.Println("Almacenando bloque para serie:", buffer.serie.Path,
		"Tiempo inicio:", tiempoInicio,
		"Tiempo final:", tiempoFinal,
		"Mediciones:", len(mediciones),
		"Tamaño comprimido:", len(bloqueFinal))

	// Escribir directamente a PebbleDB (sin serialización)
	key := generarClaveDatos(buffer.serie.SerieId, buffer.serie.TipoDatos, tiempoInicio, tiempoFinal)
	err := me.db.Set(key, bloqueFinal, pebble.Sync)
	if err != nil {
		fmt.Printf("Error al escribir datos para serie %s: %v\n", buffer.serie.Path, err)
	}
}

// ConsultarUltimoPunto obtiene la última medición registrada para una serie
func (me *ManagerEdge) ConsultarUltimoPunto(nombreSerie string) (tipos.Medicion, error) {
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
		return tipos.Medicion{}, fmt.Errorf("serie no encontrada: %s", nombreSerie)
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
		return tipos.Medicion{}, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Ir al último elemento
	if !iter.Last() {
		return tipos.Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", nombreSerie)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("error al descomprimir último bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return tipos.Medicion{}, fmt.Errorf("bloque vacío para serie: %s", nombreSerie)
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
func (me *ManagerEdge) ConsultarPrimerPunto(nombreSerie string) (tipos.Medicion, error) {
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("serie no encontrada: %s", nombreSerie)
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
		return tipos.Medicion{}, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Ir al primer elemento
	if !iter.First() {
		return tipos.Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", nombreSerie)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("error al descomprimir primer bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return tipos.Medicion{}, fmt.Errorf("bloque vacío para serie: %s", nombreSerie)
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

func (me *ManagerEdge) AgregarRegla(regla *Regla) error {
	return me.MotorReglas.AgregarRegla(regla)
}

func (me *ManagerEdge) EliminarRegla(id string) error {
	return me.MotorReglas.EliminarRegla(id)
}

func (me *ManagerEdge) ActualizarRegla(regla *Regla) error {
	return me.MotorReglas.ActualizarRegla(regla)
}

func (me *ManagerEdge) RegistrarEjecutor(tipoAccion string, ejecutor EjecutorAccion) error {
	return me.MotorReglas.RegistrarEjecutor(tipoAccion, ejecutor)
}

func (me *ManagerEdge) ProcesarDatoRegla(serie string, valor float64, timestamp time.Time) error {
	return me.MotorReglas.ProcesarDato(serie, valor, timestamp)
}

func (me *ManagerEdge) ListarReglas() map[string]*Regla {
	return me.MotorReglas.ListarReglas()
}

func (me *ManagerEdge) HabilitarMotorReglas(habilitado bool) {
	me.MotorReglas.Habilitar(habilitado)
}

func (me *ManagerEdge) EstadoConexion() string {
	estado := "local"

	if me.fdbDatabase != nil {
		estado += " + fdb"
	}

	if me.Cluster != nil {
		estado += " + " + me.Cluster.EstadoConexion()
	}

	return estado
}

func (me *ManagerEdge) ObtenerUltimaSincronizacion() time.Time {
	if me.Cluster != nil {
		return me.Cluster.ObtenerUltimaSincronizacion()
	}
	return time.Time{}
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


// Migrar a FoundationDB migra todas las series y datos a FoundationDB
func (me *ManagerEdge) Migrar() error {
	// si FoundationDB no está configurado, retornar error
	if me.fdbDatabase == nil {
		return fmt.Errorf("FoundationDB no está configurado o iniciado en este nodo edge")
	}
	
	// Iterar sobre los datos en PebbleDB y migrar a FoundationDB
	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("data/"),
		UpperBound: []byte("data0"),
	})
	if err != nil {
		return fmt.Errorf("error al crear iterador para migración: %v", err)
	}
	defer iter.Close()
	
	for iter.First(); iter.Valid(); iter.Next() {
		// Obtener clave y valor
		clave := iter.Key()
		valor := iter.Value()

		// copiar a FoundationDB
		// Crear una transacción en FoundationDB
		_, err := me.fdbDatabase.Transact(func(tr fdb.Transaction) (interface{}, error) {
			// Usar la misma clave y valor en FoundationDB
			tr.Set(fdb.Key(clave), valor)
			return nil, nil
		})
		// Verificar error de la transacción
		if err != nil {
			return fmt.Errorf("error al migrar dato a FoundationDB: %v", err)
		}
		
		// Borrar la entrada de PebbleDB después de migrar
		err = me.db.Delete(clave, pebble.Sync)
		if err != nil {
			return fmt.Errorf("error al borrar dato migrado de PebbleDB: %v", err)
		}
	}

	// Verificar errores del iterador
	if err := iter.Error(); err != nil {
		return fmt.Errorf("error durante la iteración para migración: %v", err)
	}

	log.Printf("Migración a FoundationDB completada exitosamente")
	return nil
}
