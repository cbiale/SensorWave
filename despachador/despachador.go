package despachador

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cbiale/sensorwave/compresor"
	"github.com/cbiale/sensorwave/tipos"
)

type ManagerDespachador struct {
	nodos       map[string]*tipos.Nodo // Mapa de nodos registrados
	mu          sync.RWMutex           // Mutex para proteger acceso concurrente a nodos
	s3          tipos.ClienteS3        // Cliente S3
	config      tipos.ConfiguracionS3  // Configuración de S3
	done        chan struct{}          // Canal para señalizar cierre del despachador
	clienteEdge clienteEdge            // Cliente para comunicación con edges (privado)
}

// Opciones configura la creación de un ManagerDespachador.
// El despachador SIEMPRE requiere S3 para coordinar nodos.
type Opciones struct {
	ConfigS3 tipos.ConfiguracionS3 // Siempre requerido
}

// opcionesInternas extiende Opciones con campos para testing.
// No se exporta para evitar uso en producción.
type opcionesInternas struct {
	Opciones
	clienteS3   tipos.ClienteS3 // Para inyección en tests
	clienteEdge clienteEdge     // Para inyección en tests
}

// clienteEdge define la interfaz para comunicación con nodos edge.
// Es privada para evitar que usuarios externos inyecten implementaciones.
type clienteEdge interface {
	// ConsultarRango consulta mediciones en un rango de tiempo
	ConsultarRango(ctx context.Context, direccion string, req tipos.SolicitudConsultaRango) (*tipos.RespuestaConsultaRango, error)

	// ConsultarUltimoPunto consulta el primer o último punto de una serie
	// tipo debe ser "primero" o "ultimo"
	ConsultarUltimoPunto(ctx context.Context, direccion string, req tipos.SolicitudConsultaPunto, tipo string) (*tipos.RespuestaConsultaPunto, error)

	// ConsultarAgregacion consulta una agregación simple (promedio, min, max, etc.)
	ConsultarAgregacion(ctx context.Context, direccion string, req tipos.SolicitudConsultaAgregacion) (*tipos.RespuestaConsultaAgregacion, error)

	// ConsultarAgregacionTemporal consulta agregaciones agrupadas por intervalos (downsampling)
	ConsultarAgregacionTemporal(ctx context.Context, direccion string, req tipos.SolicitudConsultaAgregacionTemporal) (*tipos.RespuestaConsultaAgregacionTemporal, error)
}

// clienteEdgeHTTP implementa clienteEdge usando HTTP directo
type clienteEdgeHTTP struct {
	httpClient *http.Client
}

// nuevoClienteEdgeHTTP crea un nuevo cliente HTTP para comunicación con edges
func nuevoClienteEdgeHTTP() *clienteEdgeHTTP {
	return &clienteEdgeHTTP{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ConsultarRango implementa clienteEdge
func (c *clienteEdgeHTTP) ConsultarRango(ctx context.Context, direccion string, req tipos.SolicitudConsultaRango) (*tipos.RespuestaConsultaRango, error) {
	// Serializar solicitud con Gob
	solicitudBytes, err := tipos.SerializarGob(req)
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud: %v", err)
	}

	// Construir URL
	url := fmt.Sprintf("http://%s/api/consulta/rango", direccion)

	// Crear request con contexto
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(solicitudBytes))
	if err != nil {
		return nil, fmt.Errorf("error creando request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	// Ejecutar request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error en request HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error del edge (status %d): %s", resp.StatusCode, string(body))
	}

	// Leer respuesta
	respuestaBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta: %v", err)
	}

	// Deserializar respuesta
	var respuesta tipos.RespuestaConsultaRango
	if err := tipos.DeserializarGob(respuestaBytes, &respuesta); err != nil {
		return nil, fmt.Errorf("error deserializando respuesta: %v", err)
	}

	return &respuesta, nil
}

// ConsultarUltimoPunto implementa clienteEdge
func (c *clienteEdgeHTTP) ConsultarUltimoPunto(ctx context.Context, direccion string, req tipos.SolicitudConsultaPunto, tipo string) (*tipos.RespuestaConsultaPunto, error) {
	// Serializar solicitud con Gob
	solicitudBytes, err := tipos.SerializarGob(req)
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud: %v", err)
	}

	// Construir URL según tipo
	url := fmt.Sprintf("http://%s/api/consulta/%s", direccion, tipo)

	// Crear request con contexto
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(solicitudBytes))
	if err != nil {
		return nil, fmt.Errorf("error creando request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	// Ejecutar request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error en request HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error del edge (status %d): %s", resp.StatusCode, string(body))
	}

	// Leer respuesta
	respuestaBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta: %v", err)
	}

	// Deserializar respuesta
	var respuesta tipos.RespuestaConsultaPunto
	if err := tipos.DeserializarGob(respuestaBytes, &respuesta); err != nil {
		return nil, fmt.Errorf("error deserializando respuesta: %v", err)
	}

	return &respuesta, nil
}

// ConsultarAgregacion implementa clienteEdge
func (c *clienteEdgeHTTP) ConsultarAgregacion(ctx context.Context, direccion string, req tipos.SolicitudConsultaAgregacion) (*tipos.RespuestaConsultaAgregacion, error) {
	// Serializar solicitud con Gob
	solicitudBytes, err := tipos.SerializarGob(req)
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud: %v", err)
	}

	// Construir URL
	url := fmt.Sprintf("http://%s/api/consulta/agregacion", direccion)

	// Crear request con contexto
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(solicitudBytes))
	if err != nil {
		return nil, fmt.Errorf("error creando request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	// Ejecutar request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error en request HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error del edge (status %d): %s", resp.StatusCode, string(body))
	}

	// Leer respuesta
	respuestaBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta: %v", err)
	}

	// Deserializar respuesta
	var respuesta tipos.RespuestaConsultaAgregacion
	if err := tipos.DeserializarGob(respuestaBytes, &respuesta); err != nil {
		return nil, fmt.Errorf("error deserializando respuesta: %v", err)
	}

	return &respuesta, nil
}

// ConsultarAgregacionTemporal implementa clienteEdge
func (c *clienteEdgeHTTP) ConsultarAgregacionTemporal(ctx context.Context, direccion string, req tipos.SolicitudConsultaAgregacionTemporal) (*tipos.RespuestaConsultaAgregacionTemporal, error) {
	// Serializar solicitud con Gob
	solicitudBytes, err := tipos.SerializarGob(req)
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud: %v", err)
	}

	// Construir URL
	url := fmt.Sprintf("http://%s/api/consulta/agregacion-temporal", direccion)

	// Crear request con contexto
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(solicitudBytes))
	if err != nil {
		return nil, fmt.Errorf("error creando request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	// Ejecutar request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error en request HTTP: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error del edge (status %d): %s", resp.StatusCode, string(body))
	}

	// Leer respuesta
	respuestaBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta: %v", err)
	}

	// Deserializar respuesta
	var respuesta tipos.RespuestaConsultaAgregacionTemporal
	if err := tipos.DeserializarGob(respuestaBytes, &respuesta); err != nil {
		return nil, fmt.Errorf("error deserializando respuesta: %v", err)
	}

	return &respuesta, nil
}

// Crear inicializa y retorna un nuevo ManagerDespachador.
// El despachador SIEMPRE requiere una configuración de S3 válida para coordinar nodos.
func Crear(opts Opciones) (*ManagerDespachador, error) {
	return crearConOpciones(opcionesInternas{Opciones: opts})
}

// crearConOpciones es la función interna que permite inyectar dependencias para testing.
// No se exporta para evitar uso en producción.
func crearConOpciones(opts opcionesInternas) (*ManagerDespachador, error) {
	cfg := opts.ConfigS3

	// Usar cliente S3 inyectado o crear uno nuevo
	var s3Client tipos.ClienteS3
	if opts.clienteS3 != nil {
		s3Client = opts.clienteS3
	} else {
		// Crear cliente S3 usando la función centralizada
		var err error
		s3Client, err = tipos.CrearClienteS3(cfg)
		if err != nil {
			return nil, err
		}
	}

	// Verificar que el bucket existe, si no, intentar crearlo
	ctx := context.TODO()
	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err != nil {
		log.Printf("El bucket %s no existe, intentando crearlo...", cfg.Bucket)
		_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(cfg.Bucket),
		})
		if err != nil {
			return nil, fmt.Errorf("error al crear bucket: %w", err)
		}
		log.Printf("Bucket %s creado exitosamente", cfg.Bucket)
	}

	// Usar cliente Edge inyectado o crear uno HTTP real
	var edgeClient clienteEdge
	if opts.clienteEdge != nil {
		edgeClient = opts.clienteEdge
	} else {
		edgeClient = nuevoClienteEdgeHTTP()
	}

	// Crear ManagerDespachador
	manager := &ManagerDespachador{
		s3:          s3Client,
		config:      cfg,
		nodos:       make(map[string]*tipos.Nodo),
		done:        make(chan struct{}),
		clienteEdge: edgeClient,
	}

	// Cargar nodos iniciales desde S3
	if err := manager.cargarNodosDesdeS3(); err != nil {
		log.Printf("Advertencia: no se pudieron cargar nodos iniciales: %v", err)
	}

	// Iniciar gorutina que sincroniza periódicamente los nodos
	go manager.monitorearNodos()

	log.Printf("Conectado a S3 en %s (bucket: %s)", cfg.Endpoint, cfg.Bucket)
	log.Printf("Despachador iniciado")
	return manager, nil
}

// Cerrar limpia los recursos del ManagerDespachador
func (m *ManagerDespachador) Cerrar() error {
	log.Printf("Cerrando despachador...")
	// Señalizar cierre
	close(m.done)
	log.Printf("Despachador cerrado exitosamente")
	return nil
}

// ListarNodos retorna una lista de nodos registrados
func (m *ManagerDespachador) ListarNodos() []tipos.Nodo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Crear una copia de los nodos para retornar
	nodosLista := make([]tipos.Nodo, 0, len(m.nodos))
	for _, nodo := range m.nodos {
		nodosLista = append(nodosLista, *nodo)
	}
	return nodosLista
}

// monitorearNodos verifica periódicamente el estado de los nodos
func (m *ManagerDespachador) monitorearNodos() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if err := m.cargarNodosDesdeS3(); err != nil {
				log.Printf("Error al cargar nodos desde S3: %v", err)
			}
		}
	}
}

// cargarNodosDesdeS3 sincroniza la lista de nodos con S3
func (m *ManagerDespachador) cargarNodosDesdeS3() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.TODO()

	// Listar todos los objetos en el bucket con prefijo "nodos/"
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(m.config.Bucket),
		Prefix: aws.String("nodos/"),
	}

	result, err := m.s3.ListObjectsV2(ctx, input)
	if err != nil {
		return fmt.Errorf("error listando nodos desde S3: %v", err)
	}

	// Actualizar la lista de nodos en memoria
	nuevosNodos := make(map[string]*tipos.Nodo)

	for _, obj := range result.Contents {
		// Obtener el objeto completo
		getInput := &s3.GetObjectInput{
			Bucket: aws.String(m.config.Bucket),
			Key:    obj.Key,
		}

		getOutput, err := m.s3.GetObject(ctx, getInput)
		if err != nil {
			log.Printf("Error obteniendo nodo %s: %v", *obj.Key, err)
			continue
		}

		// Leer el contenido
		data, err := io.ReadAll(getOutput.Body)
		getOutput.Body.Close()
		if err != nil {
			log.Printf("Error leyendo nodo %s: %v", *obj.Key, err)
			continue
		}

		// Deserializar el nodo
		var nodo tipos.Nodo
		if err := json.Unmarshal(data, &nodo); err != nil {
			log.Printf("Error deserializando nodo %s: %v", *obj.Key, err)
			continue
		}

		nuevosNodos[nodo.NodoID] = &nodo
	}

	// Reemplazar la lista de nodos
	m.nodos = nuevosNodos

	if len(nuevosNodos) > 0 {
		log.Printf("Cargados %d nodos desde S3", len(nuevosNodos))
	}

	return nil
}

// buscarNodoYSerie busca el nodo y la configuración de serie para un path dado
// Si conSerie es true, retorna también la serie encontrada
// Soporta búsqueda exacta y por prefijo parcial
func (m *ManagerDespachador) buscarNodoYSerie(nombreSerie string) (tipos.Nodo, tipos.Serie, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Primero intentar búsqueda directa por path exacto
	for _, nodo := range m.nodos {
		if serie, existe := nodo.Series[nombreSerie]; existe {
			return *nodo, serie, nil
		}
	}

	// Si no se encuentra, buscar por prefijo parcial
	for _, nodo := range m.nodos {
		for serieNombre, serie := range nodo.Series {
			if strings.HasPrefix(nombreSerie, serieNombre) || strings.HasPrefix(serieNombre, nombreSerie) {
				return *nodo, serie, nil
			}
		}
	}

	return tipos.Nodo{}, tipos.Serie{}, fmt.Errorf("serie '%s' no encontrada en ningún nodo activo", nombreSerie)
}

// ============================================================================
// CONSULTAS (S3 + Edge)
// ============================================================================

// ============================================================================
// HELPERS REUTILIZABLES
// ============================================================================

// obtenerDireccionEdge construye la dirección del edge para requests HTTP
func obtenerDireccionEdge(nodo tipos.Nodo) string {
	return fmt.Sprintf("%s:%s", nodo.DireccionIP, nodo.PuertoHTTP)
}

// consultarPuntoEdge consulta un punto (primero o último) al edge con timeout
// Retorna la medición, si se encontró, y error si hubo problemas
func (m *ManagerDespachador) consultarPuntoEdge(nodo tipos.Nodo, nombreSerie, tipoConsulta string, timeout time.Duration) (tipos.Medicion, bool, error) {
	solicitud := tipos.SolicitudConsultaPunto{
		Serie: nombreSerie,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	direccion := obtenerDireccionEdge(nodo)
	respuesta, err := m.clienteEdge.ConsultarUltimoPunto(ctx, direccion, solicitud, tipoConsulta)
	if err != nil {
		return tipos.Medicion{}, false, err
	}

	if respuesta.Error != "" {
		return tipos.Medicion{}, false, fmt.Errorf("error del edge: %s", respuesta.Error)
	}

	return respuesta.Medicion, respuesta.Encontrado, nil
}

// descargarYDescomprimirBloque descarga un bloque de S3 y lo descomprime
func (m *ManagerDespachador) descargarYDescomprimirBloque(clave string, serie tipos.Serie) ([]tipos.Medicion, error) {
	ctx := context.TODO()
	getOutput, err := m.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(m.config.Bucket),
		Key:    aws.String(clave),
	})
	if err != nil {
		return nil, fmt.Errorf("error descargando bloque %s: %v", clave, err)
	}

	datosComprimidos, err := io.ReadAll(getOutput.Body)
	getOutput.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("error leyendo bloque %s: %v", clave, err)
	}

	mediciones, err := compresor.DescomprimirBloqueSerie(
		datosComprimidos,
		serie.TipoDatos,
		serie.CompresionBytes,
		serie.CompresionBloque,
	)
	if err != nil {
		return nil, fmt.Errorf("error descomprimiendo bloque %s: %v", clave, err)
	}

	return mediciones, nil
}

// listarBloquesEnRango lista los bloques de S3 que intersectan con el rango de tiempo dado
// Retorna las claves de los objetos S3 ordenadas por tiempo
func (m *ManagerDespachador) listarBloquesEnRango(nodoID string, serieID int, inicio, fin int64) ([]string, error) {
	ctx := context.TODO()

	// Prefijo para buscar bloques: <nodoID>/data/<serieID>/
	prefijo := fmt.Sprintf("%s/data/%010d/", nodoID, serieID)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(m.config.Bucket),
		Prefix: aws.String(prefijo),
	}

	result, err := m.s3.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("error listando bloques desde S3: %v", err)
	}

	var bloquesEnRango []string

	for _, obj := range result.Contents {
		// Extraer tiempos del nombre del bloque
		// Formato: <nodoID>/data/<serieID>/<tiempoInicio>_<tiempoFin>
		clave := *obj.Key
		partes := strings.Split(clave, "/")
		if len(partes) < 4 {
			continue
		}

		nombreBloque := partes[len(partes)-1]
		tiempos := strings.Split(nombreBloque, "_")
		if len(tiempos) != 2 {
			continue
		}

		bloqueInicio, err := strconv.ParseInt(strings.TrimLeft(tiempos[0], "0"), 10, 64)
		if err != nil {
			// Si el tiempo es "00000000000000000000", ParseInt fallará
			// Intentar con el valor original
			bloqueInicio = 0
		}

		bloqueFin, err := strconv.ParseInt(strings.TrimLeft(tiempos[1], "0"), 10, 64)
		if err != nil {
			continue
		}

		// Verificar si el bloque intersecta con el rango solicitado
		// Un bloque intersecta si: bloqueInicio <= fin AND bloqueFin >= inicio
		if bloqueInicio <= fin && bloqueFin >= inicio {
			bloquesEnRango = append(bloquesEnRango, clave)
		}
	}

	// Ordenar bloques por tiempo de inicio (el nombre incluye el tiempo con padding)
	sort.Strings(bloquesEnRango)

	return bloquesEnRango, nil
}

// consultarDatosS3 descarga y descomprime bloques de S3 en el rango especificado
func (m *ManagerDespachador) consultarDatosS3(nodo tipos.Nodo, serie tipos.Serie, inicio, fin int64) ([]tipos.Medicion, error) {
	// Listar bloques en el rango
	bloques, err := m.listarBloquesEnRango(nodo.NodoID, serie.SerieId, inicio, fin)
	if err != nil {
		return nil, err
	}

	if len(bloques) == 0 {
		return []tipos.Medicion{}, nil
	}

	var todasMediciones []tipos.Medicion

	for _, clave := range bloques {
		mediciones, err := m.descargarYDescomprimirBloque(clave, serie)
		if err != nil {
			log.Printf("%v", err)
			continue
		}

		// Filtrar mediciones dentro del rango exacto
		for _, med := range mediciones {
			if med.Tiempo >= inicio && med.Tiempo <= fin {
				todasMediciones = append(todasMediciones, med)
			}
		}
	}

	return todasMediciones, nil
}

// consultarEdgeConTimeout consulta datos al edge con un timeout específico
// Retorna nil, nil si el edge no está disponible (timeout o error de conexión)
func (m *ManagerDespachador) consultarEdgeConTimeout(nodo tipos.Nodo, serie string, inicio, fin int64, timeout time.Duration) ([]tipos.Medicion, error) {
	solicitud := tipos.SolicitudConsultaRango{
		Serie:        serie,
		TiempoInicio: inicio,
		TiempoFin:    fin,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	direccion := obtenerDireccionEdge(nodo)
	respuesta, err := m.clienteEdge.ConsultarRango(ctx, direccion, solicitud)
	if err != nil {
		// Timeout o error de conexión no es crítico, el edge puede estar offline
		log.Printf("Error consultando edge %s (serie: %s): %v", nodo.NodoID, serie, err)
		return nil, nil
	}

	if respuesta.Error != "" {
		return nil, fmt.Errorf("error del edge: %s", respuesta.Error)
	}

	return respuesta.Mediciones, nil
}

// combinarResultados combina datos de S3 y edge, priorizando datos del edge en duplicados
func (m *ManagerDespachador) combinarResultados(datosS3, datosEdge []tipos.Medicion) []tipos.Medicion {
	if len(datosS3) == 0 && len(datosEdge) == 0 {
		return []tipos.Medicion{}
	}
	if len(datosS3) == 0 {
		return datosEdge
	}
	if len(datosEdge) == 0 {
		return datosS3
	}

	// Crear mapa con datos de S3
	medicionesPorTiempo := make(map[int64]tipos.Medicion)
	for _, m := range datosS3 {
		medicionesPorTiempo[m.Tiempo] = m
	}

	// Sobrescribir con datos del edge (tienen prioridad)
	for _, m := range datosEdge {
		medicionesPorTiempo[m.Tiempo] = m
	}

	// Convertir mapa a slice ordenado
	resultado := make([]tipos.Medicion, 0, len(medicionesPorTiempo))
	for _, m := range medicionesPorTiempo {
		resultado = append(resultado, m)
	}

	// Ordenar por tiempo
	sort.Slice(resultado, func(i, j int) bool {
		return resultado[i].Tiempo < resultado[j].Tiempo
	})

	return resultado
}

// ConsultarRango consulta datos combinando S3 (histórico) y edge (reciente)
// Esta función funciona incluso si el edge está offline (corte de luz/internet)
func (m *ManagerDespachador) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]tipos.Medicion, error) {
	// Buscar nodo y serie
	nodo, serie, err := m.buscarNodoYSerie(nombreSerie)
	if err != nil {
		return nil, err
	}

	inicio := tiempoInicio.UnixNano()
	fin := tiempoFin.UnixNano()

	// Canal para resultados
	type resultado struct {
		mediciones []tipos.Medicion
		err        error
		fuente     string
	}
	resultados := make(chan resultado, 2)

	// Consultar S3 en paralelo (siempre disponible)
	go func() {
		mediciones, err := m.consultarDatosS3(nodo, serie, inicio, fin)
		resultados <- resultado{mediciones: mediciones, err: err, fuente: "S3"}
	}()

	// Consultar edge con timeout de 5 segundos
	go func() {
		mediciones, err := m.consultarEdgeConTimeout(nodo, nombreSerie, inicio, fin, 5*time.Second)
		resultados <- resultado{mediciones: mediciones, err: err, fuente: "Edge"}
	}()

	// Recoger resultados
	var datosS3, datosEdge []tipos.Medicion
	var errS3, errEdge error

	for i := 0; i < 2; i++ {
		res := <-resultados
		switch res.fuente {
		case "S3":
			datosS3 = res.mediciones
			errS3 = res.err
		case "Edge":
			datosEdge = res.mediciones
			errEdge = res.err
		}
	}

	// Si S3 falla, es un error crítico (es nuestra fuente principal de históricos)
	if errS3 != nil {
		return nil, fmt.Errorf("error consultando S3: %v", errS3)
	}

	// Si el edge falla, solo logueamos (puede estar offline)
	if errEdge != nil {
		log.Printf("Advertencia: error consultando edge %s: %v (continuando con datos de S3)", nodo.NodoID, errEdge)
	}

	// Combinar resultados
	return m.combinarResultados(datosS3, datosEdge), nil
}

// ConsultarUltimoPunto busca el último punto combinando S3 y edge
func (m *ManagerDespachador) ConsultarUltimoPunto(nombreSerie string) (tipos.Medicion, error) {
	// Buscar nodo y serie
	nodo, serie, err := m.buscarNodoYSerie(nombreSerie)
	if err != nil {
		return tipos.Medicion{}, err
	}

	// Primero intentar con el edge (tiene datos más recientes)
	medicion, encontrado, err := m.consultarPuntoEdge(nodo, nombreSerie, "ultimo", 5*time.Second)
	if err == nil && encontrado {
		return medicion, nil
	}

	// Si el edge no responde, buscar en S3 el bloque más reciente
	bloques, err := m.listarBloquesEnRango(nodo.NodoID, serie.SerieId, 0, time.Now().UnixNano())
	if err != nil || len(bloques) == 0 {
		return tipos.Medicion{}, fmt.Errorf("no se encontraron datos para la serie %s", nombreSerie)
	}

	// Tomar el último bloque
	ultimoBloque := bloques[len(bloques)-1]
	mediciones, err := m.descargarYDescomprimirBloque(ultimoBloque, serie)
	if err != nil || len(mediciones) == 0 {
		return tipos.Medicion{}, fmt.Errorf("error obteniendo último bloque: %v", err)
	}

	return mediciones[len(mediciones)-1], nil
}

// ============================================================================
// CONSULTAS DE AGREGACIÓN
// ============================================================================

// calcularAgregacionSimple calcula una agregación sobre un slice de valores float64.
// Función helper interna para calcular agregaciones sobre datos combinados.
func calcularAgregacionSimple(valores []float64, agregacion tipos.TipoAgregacion) (float64, error) {
	if len(valores) == 0 {
		return 0, fmt.Errorf("no hay valores para agregar")
	}

	switch agregacion {
	case tipos.AgregacionPromedio:
		suma := 0.0
		for _, v := range valores {
			suma += v
		}
		return suma / float64(len(valores)), nil

	case tipos.AgregacionMaximo:
		max := valores[0]
		for _, v := range valores[1:] {
			if v > max {
				max = v
			}
		}
		return max, nil

	case tipos.AgregacionMinimo:
		min := valores[0]
		for _, v := range valores[1:] {
			if v < min {
				min = v
			}
		}
		return min, nil

	case tipos.AgregacionSuma:
		suma := 0.0
		for _, v := range valores {
			suma += v
		}
		return suma, nil

	case tipos.AgregacionCount:
		return float64(len(valores)), nil

	default:
		return 0, fmt.Errorf("tipo de agregación no soportado: %s", agregacion)
	}
}

// convertirMedicionesAFloat64 extrae los valores de las mediciones y los convierte a float64
func convertirMedicionesAFloat64(mediciones []tipos.Medicion) []float64 {
	valores := make([]float64, 0, len(mediciones))
	for _, m := range mediciones {
		switch v := m.Valor.(type) {
		case float64:
			valores = append(valores, v)
		case int64:
			valores = append(valores, float64(v))
		}
		// Otros tipos (bool, string) se ignoran para agregaciones numéricas
	}
	return valores
}

// ============================================================================
// HELPERS PARA WILDCARDS
// ============================================================================

// serieConNodo asocia una serie con su nodo para consultas paralelas
type serieConNodo struct {
	nodo  tipos.Nodo
	serie tipos.Serie
	path  string // Path original de la serie
}

// buscarSeriesPorPatron busca todas las series que coincidan con el patrón en todos los nodos
func (m *ManagerDespachador) buscarSeriesPorPatron(patron string) ([]serieConNodo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var resultados []serieConNodo

	for _, nodo := range m.nodos {
		for path, serie := range nodo.Series {
			if tipos.MatchPath(path, patron) {
				resultados = append(resultados, serieConNodo{
					nodo:  *nodo,
					serie: serie,
					path:  path,
				})
			}
		}
	}

	if len(resultados) == 0 {
		return nil, fmt.Errorf("no se encontraron series para el patrón: %s", patron)
	}

	return resultados, nil
}

// ConsultarAgregacion calcula una agregación simple combinando datos de S3 y edge.
// Soporta tipos de agregación: promedio, maximo, minimo, suma, count.
// Soporta wildcards en el path de la serie (ej: */temp, sensor_01/*).
func (m *ManagerDespachador) ConsultarAgregacion(
	nombreSerie string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
) (float64, error) {
	// Si es un patrón wildcard, usar flujo especial
	if tipos.EsPatronWildcard(nombreSerie) {
		return m.consultarAgregacionWildcard(nombreSerie, tiempoInicio, tiempoFin, agregacion)
	}

	// Buscar nodo y serie
	nodo, serie, err := m.buscarNodoYSerie(nombreSerie)
	if err != nil {
		return 0, err
	}

	inicio := tiempoInicio.UnixNano()
	fin := tiempoFin.UnixNano()

	// Canal para resultados
	type resultado struct {
		mediciones []tipos.Medicion
		err        error
		fuente     string
	}
	resultados := make(chan resultado, 2)

	// Consultar S3 en paralelo (siempre disponible)
	go func() {
		mediciones, err := m.consultarDatosS3(nodo, serie, inicio, fin)
		resultados <- resultado{mediciones: mediciones, err: err, fuente: "S3"}
	}()

	// Consultar edge con timeout de 5 segundos
	go func() {
		mediciones, err := m.consultarEdgeConTimeout(nodo, nombreSerie, inicio, fin, 5*time.Second)
		resultados <- resultado{mediciones: mediciones, err: err, fuente: "Edge"}
	}()

	// Recoger resultados
	var datosS3, datosEdge []tipos.Medicion
	var errS3, errEdge error

	for i := 0; i < 2; i++ {
		res := <-resultados
		switch res.fuente {
		case "S3":
			datosS3 = res.mediciones
			errS3 = res.err
		case "Edge":
			datosEdge = res.mediciones
			errEdge = res.err
		}
	}

	// Si S3 falla, es un error crítico
	if errS3 != nil {
		return 0, fmt.Errorf("error consultando S3: %v", errS3)
	}

	// Si el edge falla, solo logueamos
	if errEdge != nil {
		log.Printf("Advertencia: error consultando edge %s: %v (continuando con datos de S3)", nodo.NodoID, errEdge)
	}

	// Combinar resultados
	datosCombinados := m.combinarResultados(datosS3, datosEdge)

	if len(datosCombinados) == 0 {
		return 0, fmt.Errorf("no se encontraron datos para la serie %s en el rango especificado", nombreSerie)
	}

	// Convertir a float64 y calcular agregación
	valores := convertirMedicionesAFloat64(datosCombinados)
	if len(valores) == 0 {
		return 0, fmt.Errorf("no hay valores numéricos para agregar en la serie %s", nombreSerie)
	}

	return calcularAgregacionSimple(valores, agregacion)
}

// consultarAgregacionWildcard implementa la lógica de agregación para patrones wildcard.
// Busca todas las series que coincidan con el patrón, consulta cada una en paralelo,
// y combina todos los valores para calcular la agregación.
func (m *ManagerDespachador) consultarAgregacionWildcard(
	patron string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
) (float64, error) {
	// Buscar todas las series que coincidan con el patrón
	seriesEncontradas, err := m.buscarSeriesPorPatron(patron)
	if err != nil {
		return 0, err
	}

	inicio := tiempoInicio.UnixNano()
	fin := tiempoFin.UnixNano()

	// Canal para recoger resultados de todas las consultas
	type resultadoSerie struct {
		mediciones []tipos.Medicion
		errS3      error
		errEdge    error
		path       string
	}
	resultados := make(chan resultadoSerie, len(seriesEncontradas))

	// Consultar cada serie en paralelo (S3 + edge)
	for _, sn := range seriesEncontradas {
		go func(sn serieConNodo) {
			var datosS3, datosEdge []tipos.Medicion
			var errS3, errEdge error

			// Consultar S3
			datosS3, errS3 = m.consultarDatosS3(sn.nodo, sn.serie, inicio, fin)

			// Consultar edge
			datosEdge, errEdge = m.consultarEdgeConTimeout(sn.nodo, sn.path, inicio, fin, 5*time.Second)

			resultados <- resultadoSerie{
				mediciones: m.combinarResultados(datosS3, datosEdge),
				errS3:      errS3,
				errEdge:    errEdge,
				path:       sn.path,
			}
		}(sn)
	}

	// Recoger todos los resultados y combinar valores
	var todosValores []float64
	var erroresS3 []string

	for i := 0; i < len(seriesEncontradas); i++ {
		res := <-resultados

		// Registrar errores de S3 (críticos)
		if res.errS3 != nil {
			erroresS3 = append(erroresS3, fmt.Sprintf("%s: %v", res.path, res.errS3))
		}

		// Los errores de edge solo se loguean (el edge puede estar offline)
		if res.errEdge != nil {
			log.Printf("Advertencia: error consultando edge para serie %s: %v", res.path, res.errEdge)
		}

		// Extraer valores de las mediciones
		valores := convertirMedicionesAFloat64(res.mediciones)
		todosValores = append(todosValores, valores...)
	}

	// Si hubo errores de S3 en todas las series, reportar
	if len(erroresS3) == len(seriesEncontradas) {
		return 0, fmt.Errorf("error consultando S3 para todas las series: %v", erroresS3)
	}

	if len(todosValores) == 0 {
		return 0, fmt.Errorf("no se encontraron datos para el patrón %s en el rango especificado", patron)
	}

	return calcularAgregacionSimple(todosValores, agregacion)
}

// ConsultarAgregacionTemporal calcula agregaciones agrupadas por intervalos de tiempo (downsampling).
// Combina datos de S3 y edge, luego agrupa por intervalos del tamaño especificado.
// Soporta wildcards en el path de la serie (ej: */temp, sensor_01/*).
func (m *ManagerDespachador) ConsultarAgregacionTemporal(
	nombreSerie string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
	intervalo time.Duration,
) ([]tipos.ResultadoAgregacionTemporal, error) {
	if intervalo <= 0 {
		return nil, fmt.Errorf("intervalo debe ser mayor a cero")
	}

	// Si es un patrón wildcard, usar flujo especial
	if tipos.EsPatronWildcard(nombreSerie) {
		return m.consultarAgregacionTemporalWildcard(nombreSerie, tiempoInicio, tiempoFin, agregacion, intervalo)
	}

	// Buscar nodo y serie
	nodo, serie, err := m.buscarNodoYSerie(nombreSerie)
	if err != nil {
		return nil, err
	}

	inicio := tiempoInicio.UnixNano()
	fin := tiempoFin.UnixNano()

	// Canal para resultados
	type resultado struct {
		mediciones []tipos.Medicion
		err        error
		fuente     string
	}
	resultados := make(chan resultado, 2)

	// Consultar S3 en paralelo
	go func() {
		mediciones, err := m.consultarDatosS3(nodo, serie, inicio, fin)
		resultados <- resultado{mediciones: mediciones, err: err, fuente: "S3"}
	}()

	// Consultar edge con timeout
	go func() {
		mediciones, err := m.consultarEdgeConTimeout(nodo, nombreSerie, inicio, fin, 5*time.Second)
		resultados <- resultado{mediciones: mediciones, err: err, fuente: "Edge"}
	}()

	// Recoger resultados
	var datosS3, datosEdge []tipos.Medicion
	var errS3, errEdge error

	for i := 0; i < 2; i++ {
		res := <-resultados
		switch res.fuente {
		case "S3":
			datosS3 = res.mediciones
			errS3 = res.err
		case "Edge":
			datosEdge = res.mediciones
			errEdge = res.err
		}
	}

	if errS3 != nil {
		return nil, fmt.Errorf("error consultando S3: %v", errS3)
	}

	if errEdge != nil {
		log.Printf("Advertencia: error consultando edge %s: %v (continuando con datos de S3)", nodo.NodoID, errEdge)
	}

	// Combinar resultados
	datosCombinados := m.combinarResultados(datosS3, datosEdge)

	if len(datosCombinados) == 0 {
		return nil, fmt.Errorf("no se encontraron datos para la serie %s en el rango especificado", nombreSerie)
	}

	// Usar función helper para agrupar y calcular
	return m.agruparYCalcularAgregacion(datosCombinados, tiempoInicio, agregacion, intervalo, nombreSerie)
}

// consultarAgregacionTemporalWildcard implementa la lógica de agregación temporal para patrones wildcard.
// Busca todas las series que coincidan con el patrón, consulta cada una en paralelo,
// y agrupa todos los valores por buckets temporales para calcular la agregación.
func (m *ManagerDespachador) consultarAgregacionTemporalWildcard(
	patron string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
	intervalo time.Duration,
) ([]tipos.ResultadoAgregacionTemporal, error) {
	// Buscar todas las series que coincidan con el patrón
	seriesEncontradas, err := m.buscarSeriesPorPatron(patron)
	if err != nil {
		return nil, err
	}

	inicio := tiempoInicio.UnixNano()
	fin := tiempoFin.UnixNano()

	// Canal para recoger resultados de todas las consultas
	type resultadoSerie struct {
		mediciones []tipos.Medicion
		errS3      error
		errEdge    error
		path       string
	}
	resultados := make(chan resultadoSerie, len(seriesEncontradas))

	// Consultar cada serie en paralelo (S3 + edge)
	for _, sn := range seriesEncontradas {
		go func(sn serieConNodo) {
			var datosS3, datosEdge []tipos.Medicion
			var errS3, errEdge error

			// Consultar S3
			datosS3, errS3 = m.consultarDatosS3(sn.nodo, sn.serie, inicio, fin)

			// Consultar edge
			datosEdge, errEdge = m.consultarEdgeConTimeout(sn.nodo, sn.path, inicio, fin, 5*time.Second)

			resultados <- resultadoSerie{
				mediciones: m.combinarResultados(datosS3, datosEdge),
				errS3:      errS3,
				errEdge:    errEdge,
				path:       sn.path,
			}
		}(sn)
	}

	// Recoger todos los resultados y combinar mediciones
	var todasMediciones []tipos.Medicion
	var erroresS3 []string

	for i := 0; i < len(seriesEncontradas); i++ {
		res := <-resultados

		// Registrar errores de S3 (críticos)
		if res.errS3 != nil {
			erroresS3 = append(erroresS3, fmt.Sprintf("%s: %v", res.path, res.errS3))
		}

		// Los errores de edge solo se loguean (el edge puede estar offline)
		if res.errEdge != nil {
			log.Printf("Advertencia: error consultando edge para serie %s: %v", res.path, res.errEdge)
		}

		// Agregar mediciones al conjunto total
		todasMediciones = append(todasMediciones, res.mediciones...)
	}

	// Si hubo errores de S3 en todas las series, reportar
	if len(erroresS3) == len(seriesEncontradas) {
		return nil, fmt.Errorf("error consultando S3 para todas las series: %v", erroresS3)
	}

	if len(todasMediciones) == 0 {
		return nil, fmt.Errorf("no se encontraron datos para el patrón %s en el rango especificado", patron)
	}

	// Usar función helper para agrupar y calcular
	return m.agruparYCalcularAgregacion(todasMediciones, tiempoInicio, agregacion, intervalo, patron)
}

// agruparYCalcularAgregacion agrupa mediciones por buckets temporales y calcula la agregación.
// Función helper reutilizada por consultas normales y wildcards.
func (m *ManagerDespachador) agruparYCalcularAgregacion(
	mediciones []tipos.Medicion,
	tiempoInicio time.Time,
	agregacion tipos.TipoAgregacion,
	intervalo time.Duration,
	nombreSerie string, // Para mensajes de error
) ([]tipos.ResultadoAgregacionTemporal, error) {
	// Agrupar mediciones por buckets temporales
	buckets := make(map[int64][]float64)
	intervaloNanos := intervalo.Nanoseconds()

	for _, med := range mediciones {
		// Calcular bucket al que pertenece esta medición
		bucketInicio := tiempoInicio.UnixNano() + ((med.Tiempo-tiempoInicio.UnixNano())/intervaloNanos)*intervaloNanos

		// Convertir valor a float64
		var valorFloat float64
		switch v := med.Valor.(type) {
		case float64:
			valorFloat = v
		case int64:
			valorFloat = float64(v)
		default:
			continue // Ignorar valores no numéricos
		}

		buckets[bucketInicio] = append(buckets[bucketInicio], valorFloat)
	}

	// Calcular agregación para cada bucket
	var resultadosAgregacion []tipos.ResultadoAgregacionTemporal

	// Ordenar buckets por tiempo
	var bucketKeys []int64
	for k := range buckets {
		bucketKeys = append(bucketKeys, k)
	}
	sort.Slice(bucketKeys, func(i, j int) bool {
		return bucketKeys[i] < bucketKeys[j]
	})

	for _, bucketInicio := range bucketKeys {
		valores := buckets[bucketInicio]
		if len(valores) == 0 {
			continue
		}

		valorAgregado, err := calcularAgregacionSimple(valores, agregacion)
		if err != nil {
			continue
		}

		resultadosAgregacion = append(resultadosAgregacion, tipos.ResultadoAgregacionTemporal{
			Tiempo: time.Unix(0, bucketInicio),
			Valor:  valorAgregado,
		})
	}

	if len(resultadosAgregacion) == 0 {
		return nil, fmt.Errorf("no hay valores numéricos para agregar en la serie %s", nombreSerie)
	}

	return resultadosAgregacion, nil
}
