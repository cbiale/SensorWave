package despachador

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_nats"
	"github.com/cockroachdb/pebble"
)

type Nodo struct {
	ID              string            `json:"id"`
	Direccion       string            `json:"direccion"`
	Activo          bool              `json:"activo"`
	Series          map[string]string `json:"series"`
	UltimoHeartbeat time.Time         `json:"ultimo_heartbeat"`
}

type ManagerDespachador struct {
	db      *pebble.DB
	cliente *sw_cliente.ClienteNATS
	nodos   map[string]*Nodo
	mu      sync.RWMutex
	done    chan struct{}
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

func CrearDespachador(nombre string, direccionNATS string, puertoNATS string) (*ManagerDespachador, error) {
	db, err := pebble.Open(nombre, &pebble.Options{})
	if err != nil {
		return nil, err
	}

	cliente, err := sw_cliente.Conectar(direccionNATS, puertoNATS)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("error crítico: despachador requiere conexión NATS: %v", err)
	}

	manager := &ManagerDespachador{
		db:      db,
		cliente: cliente,
		nodos:   make(map[string]*Nodo),
		done:    make(chan struct{}),
	}

	if err := manager.cargarNodos(); err != nil {
		db.Close()
		return nil, err
	}

	go manager.escucharSuscripciones()
	go manager.escucharNuevasSeries()
	go manager.escucharHeartbeats()
	go manager.monitorearNodosInactivos()

	log.Printf("Despachador iniciado correctamente y escuchando en NATS")
	return manager, nil
}

func (m *ManagerDespachador) cargarNodos() error {
	iter, err := m.db.NewIter(nil)
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		var nodo Nodo
		if err := json.Unmarshal(iter.Value(), &nodo); err != nil {
			log.Printf("Error al deserializar nodo: %v", err)
			continue
		}
		m.nodos[string(iter.Key())] = &nodo
	}

	return iter.Error()
}

func (m *ManagerDespachador) Cerrar() error {
	close(m.done)
	if m.cliente != nil {
		m.cliente.Desconectar()
	}
	if m.db != nil {
		return m.db.Close()
	}
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

		nodoBytes, err := json.Marshal(nodo)
		if err == nil {
			m.db.Set([]byte(suscripcion.ID), nodoBytes, pebble.Sync)
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
			nodo.Series[nuevaSerie.Path] = string(rune(nuevaSerie.SerieID))
			nodo.UltimoHeartbeat = time.Now()

			nodoBytes, err := json.Marshal(nodo)
			if err == nil {
				m.db.Set([]byte(nuevaSerie.NodeID), nodoBytes, pebble.Sync)
			}
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

			nodoBytes, err := json.Marshal(nodo)
			if err == nil {
				m.db.Set([]byte(heartbeat.NodeID), nodoBytes, pebble.Sync)
			}
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

						nodoBytes, err := json.Marshal(nodo)
						if err == nil {
							m.db.Set([]byte(nodo.ID), nodoBytes, pebble.Sync)
						}
					}
				}
			}
			m.mu.Unlock()
		}
	}
}

func (m *ManagerDespachador) ListarNodos() map[string]*Nodo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copia := make(map[string]*Nodo)
	for id, nodo := range m.nodos {
		copia[id] = nodo
	}
	return copia
}
