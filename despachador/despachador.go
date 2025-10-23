package despachador

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_nats"
	"github.com/google/uuid"
)

type ManagerDespachador struct {
	cliente *sw_cliente.ClienteNATS
	nodos   map[string]*Nodo
	mu      sync.RWMutex
	done    chan struct{}
}

type Nodo struct {
	ID              string            `json:"id"`
	Direccion       string            `json:"direccion"`
	Activo          bool              `json:"activo"`
	Series          map[string]string `json:"series"`
	UltimoHeartbeat time.Time         `json:"ultimo_heartbeat"`
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

type Medicion struct {
	Tiempo int64       `json:"tiempo"`
	Valor  interface{} `json:"valor"`
}

type SolicitudConsulta struct {
	Serie        string    `json:"serie"`
	TiempoInicio time.Time `json:"tiempo_inicio"`
	TiempoFin    time.Time `json:"tiempo_fin"`
}

type RespuestaConsulta struct {
	Mediciones []Medicion `json:"mediciones"`
	Error      string     `json:"error,omitempty"`
}

func Crear(direccionNATS string, puertoNATS string) (*ManagerDespachador, error) {
	cliente, err := sw_cliente.Conectar(direccionNATS, puertoNATS)
	if err != nil {
		return nil, fmt.Errorf("error crítico: despachador requiere conexión NATS: %v", err)
	}

	manager := &ManagerDespachador{
		cliente: cliente,
		nodos:   make(map[string]*Nodo),
		done:    make(chan struct{}),
	}

	go manager.escucharSuscripciones()
	go manager.escucharNuevasSeries()
	go manager.escucharHeartbeats()
	go manager.monitorearNodosInactivos()

	log.Printf("Despachador iniciado y escuchando en NATS")
	return manager, nil
}

func (m *ManagerDespachador) Cerrar() error {
	log.Printf("Cerrando despachador...")
	close(m.done)
	if m.cliente != nil {
		m.cliente.Desconectar()
	}
	log.Printf("Despachador cerrado exitosamente")
	return nil
}

func (m *ManagerDespachador) escucharSuscripciones() {
	m.cliente.Suscribir("despachador.suscripcion", func(topico string, payload interface{}) {
		select {
		case <-m.done:
			return
		default:
		}

		payloadBytes, ok := payload.([]byte)
		if !ok {
			log.Printf("Error: payload no es []byte")
			return
		}

		var suscripcion SuscripcionNodo
		if err := json.Unmarshal(payloadBytes, &suscripcion); err != nil {
			log.Printf("Error al deserializar suscripción: %v", err)
			return
		}

		m.mu.Lock()
		nodo, existe := m.nodos[suscripcion.ID]
		if !existe {
			nodo = &Nodo{
				ID:              suscripcion.ID,
				Activo:          true,
				Series:          suscripcion.Series,
				UltimoHeartbeat: time.Now(),
			}
			m.nodos[suscripcion.ID] = nodo
		} else {
			nodo.Series = suscripcion.Series
			nodo.Activo = true
			nodo.UltimoHeartbeat = time.Now()
		}
		m.mu.Unlock()

		log.Printf("Nodo suscrito: %s con %d series", suscripcion.ID, len(suscripcion.Series))
	})
}

func (m *ManagerDespachador) escucharNuevasSeries() {
	m.cliente.Suscribir("despachador.nueva_serie", func(topico string, payload interface{}) {
		select {
		case <-m.done:
			return
		default:
		}

		payloadBytes, ok := payload.([]byte)
		if !ok {
			log.Printf("Error: payload no es []byte")
			return
		}

		var nuevaSerie NuevaSerie
		if err := json.Unmarshal(payloadBytes, &nuevaSerie); err != nil {
			log.Printf("Error al deserializar nueva serie: %v", err)
			return
		}

		m.mu.Lock()
		if nodo, existe := m.nodos[nuevaSerie.NodeID]; existe {
			if nodo.Series == nil {
				nodo.Series = make(map[string]string)
			}
			nodo.Series[nuevaSerie.Path] = fmt.Sprintf("%d", nuevaSerie.SerieID)
			nodo.UltimoHeartbeat = time.Now()
		}
		m.mu.Unlock()

		log.Printf("Nueva serie registrada: %s en nodo %s", nuevaSerie.Path, nuevaSerie.NodeID)
	})
}

func (m *ManagerDespachador) escucharHeartbeats() {
	m.cliente.Suscribir("despachador.heartbeat", func(topico string, payload interface{}) {
		select {
		case <-m.done:
			return
		default:
		}

		payloadBytes, ok := payload.([]byte)
		if !ok {
			log.Printf("Error: payload no es []byte")
			return
		}

		var heartbeat Heartbeat
		if err := json.Unmarshal(payloadBytes, &heartbeat); err != nil {
			log.Printf("Error al deserializar heartbeat: %v", err)
			return
		}

		m.mu.Lock()
		if nodo, existe := m.nodos[heartbeat.NodeID]; existe {
			nodo.Activo = heartbeat.Activo
			nodo.UltimoHeartbeat = heartbeat.Timestamp
		}
		m.mu.Unlock()

		log.Printf("Heartbeat recibido de nodo %s", heartbeat.NodeID)
	})
}

func (m *ManagerDespachador) monitorearNodosInactivos() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.mu.Lock()
			for _, nodo := range m.nodos {
				if time.Since(nodo.UltimoHeartbeat) > 2*time.Minute {
					if nodo.Activo {
						nodo.Activo = false
						log.Printf("Nodo %s marcado como INACTIVO (último heartbeat: %s)",
							nodo.ID, nodo.UltimoHeartbeat.Format(time.RFC3339))
					}
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *ManagerDespachador) ListarNodos() map[string]Nodo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copia := make(map[string]Nodo, len(m.nodos))
	for id, nodo := range m.nodos {
		seriesCopy := make(map[string]string, len(nodo.Series))
		for k, v := range nodo.Series {
			seriesCopy[k] = v
		}
		copia[id] = Nodo{
			ID:              nodo.ID,
			Direccion:       nodo.Direccion,
			Activo:          nodo.Activo,
			Series:          seriesCopy,
			UltimoHeartbeat: nodo.UltimoHeartbeat,
		}
	}
	return copia
}

func (m *ManagerDespachador) buscarNodoPorSerie(nombreSerie string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, nodo := range m.nodos {
		if !nodo.Activo {
			continue
		}
		if _, existe := nodo.Series[nombreSerie]; existe {
			return nodo.ID, nil
		}
	}

	return "", fmt.Errorf("serie '%s' no encontrada en ningún nodo activo", nombreSerie)
}

func (m *ManagerDespachador) ejecutarConsulta(nombreSerie, tipoConsulta string, solicitud interface{}, timeout time.Duration, parser func([]byte) (interface{}, error)) (interface{}, error) {
	nodeID, err := m.buscarNodoPorSerie(nombreSerie)
	if err != nil {
		return nil, err
	}

	payloadSolicitud, err := json.Marshal(solicitud)
	if err != nil {
		return nil, fmt.Errorf("error al serializar solicitud: %v", err)
	}

	topico := fmt.Sprintf("nodo.%s.consulta.%s", nodeID, tipoConsulta)

	respuestaCanal := make(chan []byte, 1)
	errorCanal := make(chan error, 1)

	topicoRespuesta := fmt.Sprintf("despachador.respuesta.%s", generarIDPeticion())

	m.cliente.Suscribir(topicoRespuesta, func(topico string, payload interface{}) {
		if payloadBytes, ok := payload.([]byte); ok {
			respuestaCanal <- payloadBytes
		} else {
			errorCanal <- fmt.Errorf("payload inválido")
		}
	})

	m.cliente.Publicar(topico, payloadSolicitud)

	select {
	case respuestaBytes := <-respuestaCanal:
		m.cliente.Desuscribir(topicoRespuesta)
		return parser(respuestaBytes)

	case err := <-errorCanal:
		m.cliente.Desuscribir(topicoRespuesta)
		return nil, err

	case <-time.After(timeout):
		m.cliente.Desuscribir(topicoRespuesta)
		return nil, fmt.Errorf("timeout esperando respuesta del nodo %s", nodeID)
	}
}

func (m *ManagerDespachador) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]Medicion, error) {
	solicitud := SolicitudConsulta{
		Serie:        nombreSerie,
		TiempoInicio: tiempoInicio,
		TiempoFin:    tiempoFin,
	}

	parser := func(data []byte) (interface{}, error) {
		var respuesta RespuestaConsulta
		if err := json.Unmarshal(data, &respuesta); err != nil {
			return nil, fmt.Errorf("error al deserializar respuesta: %v", err)
		}
		if respuesta.Error != "" {
			return nil, fmt.Errorf("error del nodo: %s", respuesta.Error)
		}
		return respuesta.Mediciones, nil
	}

	resultado, err := m.ejecutarConsulta(nombreSerie, "rango", solicitud, 30*time.Second, parser)
	if err != nil {
		return nil, err
	}
	return resultado.([]Medicion), nil
}

func (m *ManagerDespachador) ConsultarUltimoPunto(nombreSerie string) (Medicion, error) {
	solicitud := map[string]string{"serie": nombreSerie}

	parser := func(data []byte) (interface{}, error) {
		var medicion Medicion
		if err := json.Unmarshal(data, &medicion); err != nil {
			return nil, fmt.Errorf("error al deserializar respuesta: %v", err)
		}
		return medicion, nil
	}

	resultado, err := m.ejecutarConsulta(nombreSerie, "ultimo", solicitud, 10*time.Second, parser)
	if err != nil {
		return Medicion{}, err
	}
	return resultado.(Medicion), nil
}

func (m *ManagerDespachador) ConsultarPrimerPunto(nombreSerie string) (Medicion, error) {
	solicitud := map[string]string{"serie": nombreSerie}

	parser := func(data []byte) (interface{}, error) {
		var medicion Medicion
		if err := json.Unmarshal(data, &medicion); err != nil {
			return nil, fmt.Errorf("error al deserializar respuesta: %v", err)
		}
		return medicion, nil
	}

	resultado, err := m.ejecutarConsulta(nombreSerie, "primero", solicitud, 10*time.Second, parser)
	if err != nil {
		return Medicion{}, err
	}
	return resultado.(Medicion), nil
}

func generarIDPeticion() string {
	return uuid.New().String()
}
