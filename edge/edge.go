package edge

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/nats-io/nats.go"
)

type OperacionEscritura struct {
	SerieId      int
	TiempoInicio int64
	TiempoFinal  int64
	Datos        []byte
	NombreSerie  string
}

type ManagerEdge struct {
	db          *pebble.DB    // Conexión a PebbleDB
	cache       *Cache        // Cache para configuraciones de series
	buffers     sync.Map      // Buffers por serie (thread-safe map)
	done        chan struct{} // Canal para señalar cierre
	counter     int           // Contador para serie_id
	mu          sync.RWMutex  // Mutex para proteger counter
	motorReglas *MotorReglas  // Motor de reglas integrado
	natsConn    *nats.Conn    // Conexión NATS
	nodeID      string        // ID del nodo Edge
}

type Cache struct {
	datos map[string]Serie // Mapa para almacenar las configuraciones de series
	mu    sync.RWMutex     // Mutex para proteger el acceso concurrente
}

type Serie struct {
	SerieId          int                   // ID de la serie en la base de datos
	NombreSerie      string                // Nombre de la serie
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
	TipoNumerico   TipoDatos = "NUMERIC"
	TipoCategorico TipoDatos = "CATEGORICAL"
	TipoMixto      TipoDatos = "MIXED"
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

// Crear inicializa el ManagerEdge con PebbleDB y la cache
func Crear(nombre string) (ManagerEdge, error) {
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
		done:    make(chan struct{}),
		counter: 0,
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

	return *manager, nil
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

		// Extraer nombre de serie de la clave
		nombreSerie := strings.TrimPrefix(key, "series/")
		if nombreSerie == "" {
			continue
		}

		// Deserializar configuración de serie
		var config Serie
		buffer := bytes.NewBuffer(iter.Value())
		decoder := gob.NewDecoder(buffer)
		if err := decoder.Decode(&config); err != nil {
			continue // Skip series con error de deserialización
		}

		// Agregar al cache
		me.cache.mu.Lock()
		me.cache.datos[nombreSerie] = config
		me.cache.mu.Unlock()

		// Crear buffer y goroutine para cada serie
		serieBuffer := &SerieBuffer{
			datos:      make([]Medicion, config.TamañoBloque),
			serie:      config,
			indice:     0,
			done:       make(chan struct{}),
			datosCanal: make(chan Medicion, 100),
		}

		me.buffers.Store(nombreSerie, serieBuffer)
		go me.manejarBuffer(serieBuffer)
	}

	return iter.Error()
}

// generarClaveSerie genera una clave PebbleDB para metadatos de serie
func generarClaveSerie(nombreSerie string) []byte {
	return []byte("series/" + nombreSerie)
}

// generarClaveDatos genera una clave PebbleDB incluyendo el tipo de datos
func generarClaveDatos(serieId int, tipoDatos TipoDatos, tiempoInicio, tiempoFin int64) []byte {
	key := fmt.Sprintf("data/%s/%010d/%020d_%020d", string(tipoDatos), serieId, tiempoInicio, tiempoFin)
	return []byte(key)
}

// serializeserie serializa una configuración de Serie a bytes
func serializeSerie(serie Serie) ([]byte, error) {
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

	return me.db.Close()
}

// CrearSerie crea una nueva serie si no existe. Si ya existe, no hace nada.
func (me *ManagerEdge) CrearSerie(config Serie) error {
	// Verificar si la serie ya existe en cache
	me.cache.mu.RLock()
	if _, exists := me.cache.datos[config.NombreSerie]; exists {
		me.cache.mu.RUnlock()
		return nil
	}
	me.cache.mu.RUnlock()

	// Verificar si existe en PebbleDB
	key := generarClaveSerie(config.NombreSerie)
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
	serieBytes, err := serializeSerie(config)
	if err != nil {
		return fmt.Errorf("error al serializar serie: %v", err)
	}

	err = me.db.Set(key, serieBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al guardar serie: %v", err)
	}

	// Actualizar cache
	me.cache.mu.Lock()
	me.cache.datos[config.NombreSerie] = config
	me.cache.mu.Unlock()

	// Crear buffer y goroutine para la nueva serie
	buffer := &SerieBuffer{
		datos:      make([]Medicion, config.TamañoBloque),
		serie:      config,
		indice:     0,
		done:       make(chan struct{}),
		datosCanal: make(chan Medicion, 100),
	}

	me.buffers.Store(config.NombreSerie, buffer)
	go me.manejarBuffer(buffer)

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
		serieBytes, err := serializeSerie(buffer.serie)
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

// extraerTipoDeClave extrae el tipo de datos de la clave del bloque
func (me *ManagerEdge) extraerTipoDeClave(key string) TipoDatos {
	parts := strings.Split(key, "/")

	// Formato: data/TIPO/XXXXXXXXXX/TTTTTTTTTTTTTTTTTTTT_TTTTTTTTTTTTTTTTTTTT
	if len(parts) == 4 {
		return TipoDatos(parts[1])
	}

	// Formato desconocido - asumir mixto
	return TipoMixto
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
		me.motorReglas.estadisticas.ReglasActivas = len(me.motorReglas.reglas)
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
	me.motorReglas.estadisticas.ReglasActivas = len(me.motorReglas.reglas)

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
	me.motorReglas.estadisticas.ReglasActivas = len(me.motorReglas.reglas)

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

// =============================================================================
// FUNCIONES NATS
// =============================================================================

// IniciarServicioNATS inicializa la conexión NATS y servicios del nodo Edge
func (me *ManagerEdge) IniciarServicioNATS(nodeID, natsURL string) error {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("error conectando a NATS: %v", err)
	}
	me.natsConn = nc
	me.nodeID = nodeID

	// Registrarse con el despachador
	registroMsg := map[string]interface{}{
		"id":        nodeID,
		"direccion": "nats://" + nodeID,
		"timestamp": time.Now(),
	}
	registroBytes, _ := json.Marshal(registroMsg)
	nc.Publish("despachador.registro", registroBytes)

	// Suscribirse a comandos del despachador
	nc.Subscribe("edge."+nodeID+".crear_serie", me.manejarCrearSerieNATS)
	nc.Subscribe("edge."+nodeID+".obtener_series", me.manejarObtenerSeriesNATS)
	nc.Subscribe("edge."+nodeID+".listar_series", me.manejarListarSeriesNATS)
	nc.Subscribe("edge."+nodeID+".insertar", me.manejarInsertarNATS)
	nc.Subscribe("edge."+nodeID+".consultar_rango", me.manejarConsultarRangoNATS)
	nc.Subscribe("edge."+nodeID+".consultar_ultimo_punto", me.manejarConsultarUltimoPuntoNATS)
	nc.Subscribe("edge."+nodeID+".consultar_primer_punto", me.manejarConsultarPrimerPuntoNATS)

	// Suscribirse a comandos de reglas
	nc.Subscribe("edge."+nodeID+".agregar_regla", me.manejarAgregarReglaNATS)
	nc.Subscribe("edge."+nodeID+".eliminar_regla", me.manejarEliminarReglaNATS)
	nc.Subscribe("edge."+nodeID+".actualizar_regla", me.manejarActualizarReglaNATS)
	nc.Subscribe("edge."+nodeID+".listar_reglas", me.manejarListarReglasNATS)
	nc.Subscribe("edge."+nodeID+".obtener_regla", me.manejarObtenerReglaNATS)
	nc.Subscribe("edge."+nodeID+".procesar_dato_regla", me.manejarProcesarDatoReglaNATS)
	nc.Subscribe("edge."+nodeID+".habilitar_motor_reglas", me.manejarHabilitarMotorReglasNATS)

	// Iniciar heartbeat periódico
	go me.enviarHeartbeat()

	log.Printf("Nodo Edge %s conectado a NATS y servicios iniciados", nodeID)
	return nil
}

// enviarHeartbeat envía heartbeats periódicos al despachador
func (me *ManagerEdge) enviarHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-me.done:
			return
		case <-ticker.C:
			heartbeat := map[string]interface{}{
				"id":        me.nodeID,
				"timestamp": time.Now(),
				"activo":    true,
			}
			heartbeatBytes, _ := json.Marshal(heartbeat)
			me.natsConn.Publish("despachador.heartbeat", heartbeatBytes)
		}
	}
}

// =============================================================================
// MANEJADORES NATS - SERIES
// =============================================================================

func (me *ManagerEdge) manejarCrearSerieNATS(m *nats.Msg) {
	var config Serie
	err := json.Unmarshal(m.Data, &config)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error deserializando config: %v", err)))
		return
	}

	err = me.CrearSerie(config)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	m.Respond([]byte("OK"))
}

func (me *ManagerEdge) manejarObtenerSeriesNATS(m *nats.Msg) {
	var request map[string]string
	json.Unmarshal(m.Data, &request)

	serie, err := me.ObtenerSeries(request["serie"])
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	resultado, _ := json.Marshal(serie)
	m.Respond(resultado)
}

func (me *ManagerEdge) manejarListarSeriesNATS(m *nats.Msg) {
	series, err := me.ListarSeries()
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	resultado, _ := json.Marshal(series)
	m.Respond(resultado)
}

func (me *ManagerEdge) manejarInsertarNATS(m *nats.Msg) {
	var request map[string]interface{}
	json.Unmarshal(m.Data, &request)

	serie := request["serie"].(string)
	tiempo := int64(request["tiempo"].(float64))
	dato := request["dato"]

	err := me.Insertar(serie, tiempo, dato)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	m.Respond([]byte("OK"))
}

func (me *ManagerEdge) manejarConsultarRangoNATS(m *nats.Msg) {
	var request map[string]interface{}
	json.Unmarshal(m.Data, &request)

	serie := request["serie"].(string)
	inicioStr := request["inicio"].(string)
	finStr := request["fin"].(string)

	inicio, _ := time.Parse(time.RFC3339, inicioStr)
	fin, _ := time.Parse(time.RFC3339, finStr)

	resultado, err := me.ConsultarRango(serie, inicio, fin)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	resultadoBytes, _ := json.Marshal(resultado)
	m.Respond(resultadoBytes)
}

func (me *ManagerEdge) manejarConsultarUltimoPuntoNATS(m *nats.Msg) {
	var request map[string]string
	json.Unmarshal(m.Data, &request)

	resultado, err := me.ConsultarUltimoPunto(request["serie"])
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	resultadoBytes, _ := json.Marshal(resultado)
	m.Respond(resultadoBytes)
}

func (me *ManagerEdge) manejarConsultarPrimerPuntoNATS(m *nats.Msg) {
	var request map[string]string
	json.Unmarshal(m.Data, &request)

	resultado, err := me.ConsultarPrimerPunto(request["serie"])
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	resultadoBytes, _ := json.Marshal(resultado)
	m.Respond(resultadoBytes)
}

// =============================================================================
// MANEJADORES NATS - REGLAS
// =============================================================================

func (me *ManagerEdge) manejarAgregarReglaNATS(m *nats.Msg) {
	var regla *Regla
	err := json.Unmarshal(m.Data, &regla)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error deserializando regla: %v", err)))
		return
	}

	err = me.AgregarRegla(regla)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	m.Respond([]byte("OK"))
}

func (me *ManagerEdge) manejarEliminarReglaNATS(m *nats.Msg) {
	var request map[string]string
	json.Unmarshal(m.Data, &request)

	err := me.EliminarRegla(request["regla_id"])
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	m.Respond([]byte("OK"))
}

func (me *ManagerEdge) manejarActualizarReglaNATS(m *nats.Msg) {
	var regla *Regla
	err := json.Unmarshal(m.Data, &regla)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error deserializando regla: %v", err)))
		return
	}

	err = me.ActualizarRegla(regla)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	m.Respond([]byte("OK"))
}

func (me *ManagerEdge) manejarListarReglasNATS(m *nats.Msg) {
	reglas := me.ListarReglas()
	resultado, _ := json.Marshal(reglas)
	m.Respond(resultado)
}

func (me *ManagerEdge) manejarObtenerReglaNATS(m *nats.Msg) {
	var request map[string]string
	json.Unmarshal(m.Data, &request)

	regla, err := me.ObtenerRegla(request["regla_id"])
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	resultado, _ := json.Marshal(regla)
	m.Respond(resultado)
}

func (me *ManagerEdge) manejarProcesarDatoReglaNATS(m *nats.Msg) {
	var request map[string]interface{}
	json.Unmarshal(m.Data, &request)

	serie := request["serie"].(string)
	valor := request["valor"].(float64)
	timestampStr := request["timestamp"].(string)
	timestamp, _ := time.Parse(time.RFC3339, timestampStr)

	err := me.ProcesarDatoRegla(serie, valor, timestamp)
	if err != nil {
		m.Respond([]byte(fmt.Sprintf("Error: %v", err)))
		return
	}

	m.Respond([]byte("OK"))
}

func (me *ManagerEdge) manejarHabilitarMotorReglasNATS(m *nats.Msg) {
	var request map[string]bool
	json.Unmarshal(m.Data, &request)

	me.HabilitarMotorReglas(request["habilitado"])
	m.Respond([]byte("OK"))
}
