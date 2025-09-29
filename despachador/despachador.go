package despachador

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cbiale/sensorwave/edge"
	"github.com/cockroachdb/pebble"
)

type TipoUbicacion string

const (
	Original TipoUbicacion = "original"
	Copia    TipoUbicacion = "copia"
	Replica  TipoUbicacion = "replica"
)

type ManagerDespachador struct {
	nodos map[string]*NodoEdge
	db    *pebble.DB
	mu    sync.RWMutex
}

type NodoEdge struct {
	ID        string
	Direccion string
	Series    map[string]TipoUbicacion
	Activo    bool
	Cliente   *edge.ManagerEdge
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
			Cliente:   nil,   // Se establecerá cuando se conecte
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
func (md *ManagerDespachador) RegistrarNodo(id, direccion string, cliente *edge.ManagerEdge) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	// Crear nuevo nodo
	nodo := &NodoEdge{
		ID:        id,
		Direccion: direccion,
		Series:    make(map[string]TipoUbicacion),
		Activo:    true,
		Cliente:   cliente,
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
// FUNCIONES DE CONSULTA DISTRIBUIDAS (PROXY A NODOS EDGE)
// =============================================================================

// CrearSerie crea una nueva serie en un nodo específico
func (md *ManagerDespachador) CrearSerie(nodeID string, config edge.Serie) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	err := nodo.Cliente.CrearSerie(config)
	if err != nil {
		return fmt.Errorf("error creando serie en nodo %s: %v", nodeID, err)
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

	if nodo.Cliente == nil {
		return edge.Serie{}, fmt.Errorf("cliente no disponible para nodo %s", nodo.ID)
	}

	return nodo.Cliente.ObtenerSeries(nombreSerie)
}

// ListarSeries lista todas las series desde un nodo específico
func (md *ManagerDespachador) ListarSeries(nodeID string) ([]string, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return nil, fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.ListarSeries()
}

// Insertar inserta una medición en un nodo específico
func (md *ManagerDespachador) Insertar(nodeID, nombreSerie string, tiempo int64, dato interface{}) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.Insertar(nombreSerie, tiempo, dato)
}

// ConsultarRango consulta un rango de tiempo desde el mejor nodo disponible
func (md *ManagerDespachador) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]edge.Medicion, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return nil, err
	}

	if nodo.Cliente == nil {
		return nil, fmt.Errorf("cliente no disponible para nodo %s", nodo.ID)
	}

	return nodo.Cliente.ConsultarRango(nombreSerie, tiempoInicio, tiempoFin)
}

// ConsultarUltimoPunto obtiene último punto desde el mejor nodo
func (md *ManagerDespachador) ConsultarUltimoPunto(nombreSerie string) (edge.Medicion, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return edge.Medicion{}, err
	}

	if nodo.Cliente == nil {
		return edge.Medicion{}, fmt.Errorf("cliente no disponible para nodo %s", nodo.ID)
	}

	return nodo.Cliente.ConsultarUltimoPunto(nombreSerie)
}

// ConsultarPrimerPunto obtiene primer punto desde el mejor nodo
func (md *ManagerDespachador) ConsultarPrimerPunto(nombreSerie string) (edge.Medicion, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, err := md.seleccionarMejorNodo(nombreSerie)
	if err != nil {
		return edge.Medicion{}, err
	}

	if nodo.Cliente == nil {
		return edge.Medicion{}, fmt.Errorf("cliente no disponible para nodo %s", nodo.ID)
	}

	return nodo.Cliente.ConsultarPrimerPunto(nombreSerie)
}

// =============================================================================
// FUNCIONES DE REGLAS DISTRIBUIDAS (PROXY A NODOS EDGE)
// =============================================================================

// AgregarRegla agrega una regla a un nodo específico
func (md *ManagerDespachador) AgregarRegla(nodeID string, regla *edge.Regla) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.AgregarRegla(regla)
}

// EliminarRegla elimina una regla de un nodo específico
func (md *ManagerDespachador) EliminarRegla(nodeID, reglaID string) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.EliminarRegla(reglaID)
}

// ActualizarRegla actualiza una regla en un nodo específico
func (md *ManagerDespachador) ActualizarRegla(nodeID string, regla *edge.Regla) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.ActualizarRegla(regla)
}

// ListarReglas lista reglas de un nodo específico
func (md *ManagerDespachador) ListarReglas(nodeID string) (map[string]*edge.Regla, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return nil, fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.ListarReglas(), nil
}

// ObtenerRegla obtiene una regla específica de un nodo
func (md *ManagerDespachador) ObtenerRegla(nodeID, reglaID string) (*edge.Regla, error) {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return nil, fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.ObtenerRegla(reglaID)
}

// ProcesarDatoRegla procesa un dato según reglas en un nodo específico
func (md *ManagerDespachador) ProcesarDatoRegla(nodeID, serie string, valor float64, timestamp time.Time) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.ProcesarDatoRegla(serie, valor, timestamp)
}

// RegistrarEjecutor registra un ejecutor de acciones en un nodo específico
func (md *ManagerDespachador) RegistrarEjecutor(nodeID, tipoAccion string, ejecutor edge.EjecutorAccion) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	return nodo.Cliente.RegistrarEjecutor(tipoAccion, ejecutor)
}

// HabilitarMotorReglas habilita/deshabilita motor de reglas en un nodo
func (md *ManagerDespachador) HabilitarMotorReglas(nodeID string, habilitado bool) error {
	md.mu.RLock()
	defer md.mu.RUnlock()

	nodo, exists := md.nodos[nodeID]
	if !exists || !nodo.Activo || nodo.Cliente == nil {
		return fmt.Errorf("nodo '%s' no disponible", nodeID)
	}

	nodo.Cliente.HabilitarMotorReglas(habilitado)
	return nil
}

