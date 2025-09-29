package despachador

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cbiale/sensorwave/edge"
	"github.com/cockroachdb/pebble"
	"github.com/nats-io/nats.go"
)

type TipoUbicacion string

const (
	Original TipoUbicacion = "original"
	Copia    TipoUbicacion = "copia"
	Replica  TipoUbicacion = "replica"
)

type ManagerDespachador struct {
	nodos    map[string]*NodoEdge
	db       *pebble.DB
	mu       sync.RWMutex
	natsConn *nats.Conn
}

type NodoEdge struct {
	ID        string
	Direccion string
	Series    map[string]TipoUbicacion
	Activo    bool
}

type InfoNodo struct {
	ID        string
	Direccion string
	Series    map[string]TipoUbicacion
	Activo    bool
}

// CrearDespachador inicializa el ManagerDespachador con PebbleDB
func CrearDespachador(nombre string) (*ManagerDespachador, error) {
	db, err := pebble.Open(nombre, &pebble.Options{})
	if err != nil {
		return nil, err
	}

	despachador := &ManagerDespachador{
		nodos: make(map[string]*NodoEdge),
		db:    db,
	}

	// Cargar nodos existentes desde PebbleDB
	err = despachador.cargarNodosExistentes()
	if err != nil {
		return nil, fmt.Errorf("error al cargar nodos: %v", err)
	}

	return despachador, nil
}

// cargarNodosExistentes carga todos los nodos desde PebbleDB
func (md *ManagerDespachador) cargarNodosExistentes() error {
	iter, err := md.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("nodos/"),
		UpperBound: []byte("nodos0"),
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if !strings.HasPrefix(key, "nodos/") {
			continue
		}

		// Extraer ID del nodo de la clave
		nodeID := strings.TrimPrefix(key, "nodos/")
		if nodeID == "" {
			continue
		}

		// Deserializar información del nodo
		var info InfoNodo
		buffer := bytes.NewBuffer(iter.Value())
		decoder := gob.NewDecoder(buffer)
		if err := decoder.Decode(&info); err != nil {
			continue
		}

		// Crear nodo (sin cliente, se conectará después)
		nodo := &NodoEdge{
			ID:        info.ID,
			Direccion: info.Direccion,
			Series:    info.Series,
			Activo:    false, // Se marcará como activo cuando se conecte
		}

		md.nodos[nodeID] = nodo
	}

	return iter.Error()
}

// generateNodoKey genera una clave PebbleDB para metadatos de nodo
func generateNodoKey(nodeID string) []byte {
	return []byte("nodos/" + nodeID)
}

// serializeNodo serializa información de un nodo a bytes
func serializeNodo(nodo *NodoEdge) ([]byte, error) {
	info := InfoNodo{
		ID:        nodo.ID,
		Direccion: nodo.Direccion,
		Series:    nodo.Series,
		Activo:    nodo.Activo,
	}

	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(info)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Cerrar cierra la conexión a PebbleDB
func (md *ManagerDespachador) Cerrar() error {
	return md.db.Close()
}

// RegistrarNodo registra un nuevo nodo edge en el despachador
func (md *ManagerDespachador) RegistrarNodo(id, direccion string) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	// Crear nuevo nodo
	nodo := &NodoEdge{
		ID:        id,
		Direccion: direccion,
		Series:    make(map[string]TipoUbicacion),
		Activo:    true,
	}

	// Guardar en PebbleDB
	key := generateNodoKey(id)
	nodoBytes, err := serializeNodo(nodo)
	if err != nil {
		return fmt.Errorf("error al serializar nodo: %v", err)
	}

	err = md.db.Set(key, nodoBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al guardar nodo: %v", err)
	}

	// Agregar al cache en memoria
	md.nodos[id] = nodo

	log.Printf("Nodo '%s' registrado en %s", id, direccion)
	return nil
}

// DesregistrarNodo elimina un nodo del despachador
func (md *ManagerDespachador) DesregistrarNodo(id string) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	if _, exists := md.nodos[id]; !exists {
		return fmt.Errorf("nodo '%s' no encontrado", id)
	}

	// Eliminar de PebbleDB
	key := generateNodoKey(id)
	err := md.db.Delete(key, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al eliminar nodo de DB: %v", err)
	}

	// Eliminar del cache
	delete(md.nodos, id)

	log.Printf("Nodo '%s' desregistrado", id)
	return nil
}

// ActualizarEstadoNodo marca un nodo como activo/inactivo
func (md *ManagerDespachador) ActualizarEstadoNodo(id string, activo bool) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	nodo, exists := md.nodos[id]
	if !exists {
		return fmt.Errorf("nodo '%s' no encontrado", id)
	}

	nodo.Activo = activo

	// Actualizar en PebbleDB
	key := generateNodoKey(id)
	nodoBytes, err := serializeNodo(nodo)
	if err != nil {
		return fmt.Errorf("error al serializar nodo: %v", err)
	}

	err = md.db.Set(key, nodoBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al actualizar nodo: %v", err)
	}

	log.Printf("Estado del nodo '%s' actualizado: activo=%v", id, activo)
	return nil
}

// AsignarSerie asigna una serie a un nodo con un tipo de ubicación específico
func (md *ManagerDespachador) AsignarSerie(nodeID, nombreSerie string, tipoUbicacion TipoUbicacion) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	nodo, exists := md.nodos[nodeID]
	if !exists {
		return fmt.Errorf("nodo '%s' no encontrado", nodeID)
	}

	// Asignar serie al nodo
	nodo.Series[nombreSerie] = tipoUbicacion

	// Actualizar en PebbleDB
	key := generateNodoKey(nodeID)
	nodoBytes, err := serializeNodo(nodo)
	if err != nil {
		return fmt.Errorf("error al serializar nodo: %v", err)
	}

	err = md.db.Set(key, nodoBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al actualizar nodo: %v", err)
	}

	log.Printf("Serie '%s' asignada al nodo '%s' como '%s'", nombreSerie, nodeID, tipoUbicacion)
	return nil
}

// DesasignarSerie remueve una serie de un nodo
func (md *ManagerDespachador) DesasignarSerie(nodeID, nombreSerie string) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	nodo, exists := md.nodos[nodeID]
	if !exists {
		return fmt.Errorf("nodo '%s' no encontrado", nodeID)
	}

	// Remover serie del nodo
	delete(nodo.Series, nombreSerie)

	// Actualizar en PebbleDB
	key := generateNodoKey(nodeID)
	nodoBytes, err := serializeNodo(nodo)
	if err != nil {
		return fmt.Errorf("error al serializar nodo: %v", err)
	}

	err = md.db.Set(key, nodoBytes, pebble.Sync)
	if err != nil {
		return fmt.Errorf("error al actualizar nodo: %v", err)
	}

	log.Printf("Serie '%s' desasignada del nodo '%s'", nombreSerie, nodeID)
	return nil
}

// obtenerNodosSerie obtiene todos los nodos que tienen una serie específica
func (md *ManagerDespachador) obtenerNodosSerie(nombreSerie string) []*NodoEdge {
	var nodos []*NodoEdge
	for _, nodo := range md.nodos {
		if _, tiene := nodo.Series[nombreSerie]; tiene && nodo.Activo {
			nodos = append(nodos, nodo)
		}
	}
	return nodos
}

// seleccionarMejorNodo selecciona el mejor nodo para una consulta basado en prioridad
func (md *ManagerDespachador) seleccionarMejorNodo(nombreSerie string) (*NodoEdge, error) {
	nodos := md.obtenerNodosSerie(nombreSerie)
	if len(nodos) == 0 {
		return nil, fmt.Errorf("no hay nodos activos con la serie '%s'", nombreSerie)
	}

	// Prioridad: Original > Copia > Replica
	var original, copia, replica *NodoEdge
	for _, nodo := range nodos {
		switch nodo.Series[nombreSerie] {
		case Original:
			if original == nil {
				original = nodo
			}
		case Copia:
			if copia == nil {
				copia = nodo
			}
		case Replica:
			if replica == nil {
				replica = nodo
			}
		}
	}

	// Retornar el mejor disponible
	if original != nil {
		return original, nil
	}
	if copia != nil {
		return copia, nil
	}
	if replica != nil {
		return replica, nil
	}

	return nodos[0], nil // Fallback al primer nodo disponible
}

// ListarNodos retorna información de todos los nodos registrados
func (md *ManagerDespachador) ListarNodos() map[string]*NodoEdge {
	md.mu.RLock()
	defer md.mu.RUnlock()

	copia := make(map[string]*NodoEdge)
	for id, nodo := range md.nodos {
		copia[id] = nodo
	}
	return copia
}

// ListarSeriesGlobal retorna todas las series conocidas en el cluster
func (md *ManagerDespachador) ListarSeriesGlobal() []string {
	md.mu.RLock()
	defer md.mu.RUnlock()

	seriesMap := make(map[string]bool)
	for _, nodo := range md.nodos {
		for serie := range nodo.Series {
			seriesMap[serie] = true
		}
	}

	var series []string
	for serie := range seriesMap {
		series = append(series, serie)
	}

	sort.Strings(series)
	return series
}

// =============================================================================
// FUNCIONES NATS
// =============================================================================

// IniciarNATS inicializa la conexión NATS del despachador
func (md *ManagerDespachador) IniciarNATS(natsURL string) error {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("error conectando a NATS: %v", err)
	}
	md.natsConn = nc

	// Suscribirse a registros de nodos
	_, err = nc.Subscribe("despachador.registro", md.manejarRegistroNodo)
	if err != nil {
		return fmt.Errorf("error suscribiéndose a despachador.registro: %v", err)
	}

	// Suscribirse a heartbeats de nodos
	_, err = nc.Subscribe("despachador.heartbeat", md.manejarHeartbeat)
	if err != nil {
		return fmt.Errorf("error suscribiéndose a despachador.heartbeat: %v", err)
	}

	log.Printf("Despachador conectado a NATS en %s", natsURL)
	return nil
}

// manejarRegistroNodo maneja el registro de nuevos nodos edge
func (md *ManagerDespachador) manejarRegistroNodo(m *nats.Msg) {
	var registroMsg struct {
		ID        string    `json:"id"`
		Direccion string    `json:"direccion"`
		Timestamp time.Time `json:"timestamp"`
	}

	err := json.Unmarshal(m.Data, &registroMsg)
	if err != nil {
		log.Printf("Error procesando registro de nodo: %v", err)
		return
	}

	// Registrar el nodo
	err = md.RegistrarNodo(registroMsg.ID, registroMsg.Direccion)
	if err != nil {
		log.Printf("Error registrando nodo %s: %v", registroMsg.ID, err)
		return
	}

	log.Printf("Nodo %s registrado exitosamente via NATS", registroMsg.ID)
}

// manejarHeartbeat maneja los heartbeats de nodos edge
func (md *ManagerDespachador) manejarHeartbeat(m *nats.Msg) {
	var heartbeatMsg struct {
		ID        string    `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Activo    bool      `json:"activo"`
	}

	err := json.Unmarshal(m.Data, &heartbeatMsg)
	if err != nil {
		log.Printf("Error procesando heartbeat: %v", err)
		return
	}

	// Actualizar estado del nodo
	err = md.ActualizarEstadoNodo(heartbeatMsg.ID, heartbeatMsg.Activo)
	if err != nil {
		log.Printf("Error actualizando estado del nodo %s: %v", heartbeatMsg.ID, err)
	}
}

// =============================================================================
// FUNCIONES DE CONSULTA DISTRIBUIDAS (PROXY A NODOS EDGE VIA NATS)
// =============================================================================

// CrearSerie crea una nueva serie en un nodo específico
func (md *ManagerDespachador) CrearSerie(nodeID string, config edge.Serie) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("error serializando config: %v", err)
	}

	// Request via NATS
	_, err = md.natsConn.Request("edge."+nodeID+".crear_serie", payload, time.Second*10)
	if err != nil {
		return fmt.Errorf("error creando serie en nodo %s via NATS: %v", nodeID, err)
	}

	// Asignar serie al nodo como original
	return md.AsignarSerie(nodeID, config.NombreSerie, Original)
}

// ObtenerSeries obtiene información de una serie desde el mejor nodo disponible
func (md *ManagerDespachador) ObtenerSeries(nombreSerie string) (edge.Serie, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return edge.Serie{}, err
	}

	// Request via NATS
	payload := map[string]string{"serie": nombreSerie}
	payloadBytes, _ := json.Marshal(payload)

	msg, err := md.natsConn.Request("edge."+nodo.ID+".obtener_series", payloadBytes, time.Second*10)
	if err != nil {
		return edge.Serie{}, fmt.Errorf("error obteniendo serie via NATS: %v", err)
	}

	var resultado edge.Serie
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// ListarSeries lista todas las series desde un nodo específico
func (md *ManagerDespachador) ListarSeries(nodeID string) ([]string, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return nil, fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Request via NATS
	msg, err := md.natsConn.Request("edge."+nodeID+".listar_series", []byte("{}"), time.Second*10)
	if err != nil {
		return nil, fmt.Errorf("error listando series via NATS: %v", err)
	}

	var resultado []string
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// Insertar inserta una medición en un nodo específico
func (md *ManagerDespachador) Insertar(nodeID, nombreSerie string, tiempo int64, dato interface{}) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload := map[string]interface{}{
		"serie":  nombreSerie,
		"tiempo": tiempo,
		"dato":   dato,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error serializando payload: %v", err)
	}

	// Request via NATS
	_, err = md.natsConn.Request("edge."+nodeID+".insertar", payloadBytes, time.Second*10)
	if err != nil {
		return fmt.Errorf("error insertando via NATS: %v", err)
	}

	return nil
}

// ConsultarRango consulta un rango de tiempo desde el mejor nodo disponible
func (md *ManagerDespachador) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]edge.Medicion, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return nil, err
	}

	// Crear payload para NATS
	payload := map[string]interface{}{
		"serie":  nombreSerie,
		"inicio": tiempoInicio,
		"fin":    tiempoFin,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error serializando payload: %v", err)
	}

	// Request via NATS
	msg, err := md.natsConn.Request("edge."+nodo.ID+".consultar_rango", payloadBytes, time.Second*10)
	if err != nil {
		return nil, fmt.Errorf("error consultando rango via NATS: %v", err)
	}

	var resultado []edge.Medicion
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// ConsultarUltimoPunto obtiene último punto desde el mejor nodo
func (md *ManagerDespachador) ConsultarUltimoPunto(nombreSerie string) (edge.Medicion, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return edge.Medicion{}, err
	}

	// Request via NATS
	payload := map[string]string{"serie": nombreSerie}
	payloadBytes, _ := json.Marshal(payload)

	msg, err := md.natsConn.Request("edge."+nodo.ID+".consultar_ultimo_punto", payloadBytes, time.Second*10)
	if err != nil {
		return edge.Medicion{}, fmt.Errorf("error consultando último punto via NATS: %v", err)
	}

	var resultado edge.Medicion
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// ConsultarPrimerPunto obtiene primer punto desde el mejor nodo
func (md *ManagerDespachador) ConsultarPrimerPunto(nombreSerie string) (edge.Medicion, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return edge.Medicion{}, err
	}

	// Request via NATS
	payload := map[string]string{"serie": nombreSerie}
	payloadBytes, _ := json.Marshal(payload)

	msg, err := md.natsConn.Request("edge."+nodo.ID+".consultar_primer_punto", payloadBytes, time.Second*10)
	if err != nil {
		return edge.Medicion{}, fmt.Errorf("error consultando primer punto via NATS: %v", err)
	}

	var resultado edge.Medicion
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// =============================================================================
// FUNCIONES DE REGLAS DISTRIBUIDAS (PROXY A NODOS EDGE VIA NATS)
// =============================================================================

// AgregarRegla agrega una regla a un nodo específico
func (md *ManagerDespachador) AgregarRegla(nodeID string, regla *edge.Regla) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload, err := json.Marshal(regla)
	if err != nil {
		return fmt.Errorf("error serializando regla: %v", err)
	}

	// Request via NATS
	_, err = md.natsConn.Request("edge."+nodeID+".agregar_regla", payload, time.Second*10)
	if err != nil {
		return fmt.Errorf("error agregando regla via NATS: %v", err)
	}

	return nil
}

// EliminarRegla elimina una regla de un nodo específico
func (md *ManagerDespachador) EliminarRegla(nodeID, reglaID string) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload := map[string]string{"regla_id": reglaID}
	payloadBytes, _ := json.Marshal(payload)

	// Request via NATS
	_, err := md.natsConn.Request("edge."+nodeID+".eliminar_regla", payloadBytes, time.Second*10)
	if err != nil {
		return fmt.Errorf("error eliminando regla via NATS: %v", err)
	}

	return nil
}

// ActualizarRegla actualiza una regla en un nodo específico
func (md *ManagerDespachador) ActualizarRegla(nodeID string, regla *edge.Regla) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload, err := json.Marshal(regla)
	if err != nil {
		return fmt.Errorf("error serializando regla: %v", err)
	}

	// Request via NATS
	_, err = md.natsConn.Request("edge."+nodeID+".actualizar_regla", payload, time.Second*10)
	if err != nil {
		return fmt.Errorf("error actualizando regla via NATS: %v", err)
	}

	return nil
}

// ListarReglas lista reglas de un nodo específico
func (md *ManagerDespachador) ListarReglas(nodeID string) (map[string]*edge.Regla, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return nil, fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Request via NATS
	msg, err := md.natsConn.Request("edge."+nodeID+".listar_reglas", []byte("{}"), time.Second*10)
	if err != nil {
		return nil, fmt.Errorf("error listando reglas via NATS: %v", err)
	}

	var resultado map[string]*edge.Regla
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// ObtenerRegla obtiene una regla específica de un nodo
func (md *ManagerDespachador) ObtenerRegla(nodeID, reglaID string) (*edge.Regla, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return nil, fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload := map[string]string{"regla_id": reglaID}
	payloadBytes, _ := json.Marshal(payload)

	// Request via NATS
	msg, err := md.natsConn.Request("edge."+nodeID+".obtener_regla", payloadBytes, time.Second*10)
	if err != nil {
		return nil, fmt.Errorf("error obteniendo regla via NATS: %v", err)
	}

	var resultado *edge.Regla
	err = json.Unmarshal(msg.Data, &resultado)
	return resultado, err
}

// ProcesarDatoRegla procesa un dato según reglas en un nodo específico
func (md *ManagerDespachador) ProcesarDatoRegla(nodeID, serie string, valor float64, timestamp time.Time) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload := map[string]interface{}{
		"serie":     serie,
		"valor":     valor,
		"timestamp": timestamp,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error serializando payload: %v", err)
	}

	// Request via NATS
	_, err = md.natsConn.Request("edge."+nodeID+".procesar_dato_regla", payloadBytes, time.Second*10)
	if err != nil {
		return fmt.Errorf("error procesando dato regla via NATS: %v", err)
	}

	return nil
}

// RegistrarEjecutor registra un ejecutor de acciones en un nodo específico
// NOTA: Esta función requiere callback, no es compatible con NATS directamente
func (md *ManagerDespachador) RegistrarEjecutor(nodeID, tipoAccion string, ejecutor edge.EjecutorAccion) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Esta función requiere un callback, no es compatible con NATS directamente
	// Se podría implementar un patrón de registro diferente si es necesario
	return fmt.Errorf("RegistrarEjecutor no soportado via NATS (requiere callback)")
}

// HabilitarMotorReglas habilita/deshabilita motor de reglas en un nodo
func (md *ManagerDespachador) HabilitarMotorReglas(nodeID string, habilitado bool) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	// Crear payload para NATS
	payload := map[string]bool{"habilitado": habilitado}
	payloadBytes, _ := json.Marshal(payload)

	// Request via NATS
	_, err := md.natsConn.Request("edge."+nodeID+".habilitar_motor_reglas", payloadBytes, time.Second*10)
	if err != nil {
		return fmt.Errorf("error habilitando motor reglas via NATS: %v", err)
	}

	return nil
}
