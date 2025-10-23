package edge

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	sw_cliente "github.com/cbiale/sensorwave/middleware/cliente_nats"
)

type ClusterManager struct {
	manager         *ManagerEdge
	cliente         *sw_cliente.ClienteNATS
	direccionNATS   string
	puertoNATS      string
	nodeID          string
	reconectando    bool
	muConexion      sync.Mutex
	ultimaSincro    time.Time
	muSincronizando sync.Mutex
	done            chan struct{}
}

func nuevoClusterManager(manager *ManagerEdge, nodeID string, direccionNATS, puertoNATS string, done chan struct{}) *ClusterManager {
	return &ClusterManager{
		manager:       manager,
		nodeID:        nodeID,
		direccionNATS: direccionNATS,
		puertoNATS:    puertoNATS,
		done:          done,
	}
}

func (cm *ClusterManager) conectar() error {
	cliente, err := sw_cliente.Conectar(cm.direccionNATS, cm.puertoNATS)
	if err != nil {
		log.Printf("ADVERTENCIA: Nodo edge funcionando en modo autónomo sin NATS: %v", err)
		log.Printf("El nodo continuará operando localmente. Las funciones de cluster están deshabilitadas.")
		cm.cliente = nil
		go cm.intentarReconexionPeriodica()
		return err
	}

	cm.cliente = cliente

	if err := cm.sincronizarEstado(); err != nil {
		log.Printf("Error al sincronizar estado inicial: %v", err)
	}

	go cm.enviarHeartbeat()
	cm.iniciarListenersConsultas()

	log.Printf("Nodo edge conectado al cluster vía NATS")
	return nil
}

func (cm *ClusterManager) cerrar() {
	if cm.cliente != nil {
		cm.cliente.Desconectar()
	}
}

func (cm *ClusterManager) informarSuscripcion() error {
	if cm.cliente == nil {
		return nil
	}

	series, err := cm.manager.ListarSeries()
	if err != nil {
		return fmt.Errorf("error al listar series: %v", err)
	}

	seriesMap := make(map[string]string)
	for _, path := range series {
		serie, err := cm.manager.ObtenerSeries(path)
		if err != nil {
			continue
		}
		seriesMap[path] = fmt.Sprintf("%d", serie.SerieId)
	}

	suscripcion := SuscripcionNodo{
		ID:     cm.nodeID,
		Series: seriesMap,
	}

	payload, err := json.Marshal(suscripcion)
	if err != nil {
		return fmt.Errorf("error al serializar suscripción: %v", err)
	}

	cm.cliente.Publicar("despachador.suscripcion", payload)
	log.Printf("Suscripción informada: nodo %s con %d series", cm.nodeID, len(seriesMap))
	return nil
}

func (cm *ClusterManager) InformarNuevaSerie(path string, serieID int) error {
	if cm.cliente == nil {
		return nil
	}

	nuevaSerie := NuevaSerie{
		NodeID:  cm.nodeID,
		Path:    path,
		SerieID: serieID,
	}

	payload, err := json.Marshal(nuevaSerie)
	if err != nil {
		return fmt.Errorf("error al serializar nueva serie: %v", err)
	}

	cm.cliente.Publicar("despachador.nueva_serie", payload)
	log.Printf("Nueva serie informada: %s (ID: %d) en nodo %s", path, serieID, cm.nodeID)
	return nil
}

func (cm *ClusterManager) enviarHeartbeat() {
	if cm.cliente == nil {
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.done:
			return
		case <-ticker.C:
			if cm.cliente == nil {
				return
			}

			heartbeat := Heartbeat{
				NodeID:    cm.nodeID,
				Timestamp: time.Now(),
				Activo:    true,
			}

			payload, err := json.Marshal(heartbeat)
			if err != nil {
				log.Printf("Error al serializar heartbeat: %v", err)
				continue
			}

			cm.cliente.Publicar("despachador.heartbeat", payload)
			log.Printf("Heartbeat enviado desde nodo %s", cm.nodeID)
		}
	}
}

func (cm *ClusterManager) intentarReconexionPeriodica() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-cm.done:
			return
		case <-ticker.C:
			cm.muConexion.Lock()
			if cm.cliente != nil {
				cm.muConexion.Unlock()
				return
			}

			if cm.reconectando {
				cm.muConexion.Unlock()
				continue
			}

			cm.reconectando = true
			cm.muConexion.Unlock()

			log.Printf("Intentando reconectar a NATS...")
			cliente, err := sw_cliente.Conectar(cm.direccionNATS, cm.puertoNATS)

			cm.muConexion.Lock()
			if err != nil {
				log.Printf("Fallo al reconectar a NATS: %v", err)
				cm.reconectando = false
				cm.muConexion.Unlock()
				continue
			}

			cm.cliente = cliente
			cm.reconectando = false
			cm.muConexion.Unlock()

			log.Printf("Reconexión a NATS exitosa")

			if err := cm.sincronizarEstado(); err != nil {
				log.Printf("Error al sincronizar estado después de reconexión: %v", err)
			}

			go cm.enviarHeartbeat()
			return
		}
	}
}

func (cm *ClusterManager) sincronizarEstado() error {
	cm.muSincronizando.Lock()
	defer cm.muSincronizando.Unlock()

	if cm.cliente == nil {
		return fmt.Errorf("cliente NATS no disponible")
	}

	if err := cm.informarSuscripcion(); err != nil {
		return fmt.Errorf("error al informar suscripción: %v", err)
	}

	cm.ultimaSincro = time.Now()
	log.Printf("Estado sincronizado exitosamente para nodo %s", cm.nodeID)
	return nil
}

func (cm *ClusterManager) ManejarDesconexion() {
	cm.muConexion.Lock()
	defer cm.muConexion.Unlock()

	if cm.cliente != nil {
		cm.cliente.Desconectar()
		cm.cliente = nil
		log.Printf("Desconectado de NATS, cambiando a modo autónomo")
	}

	go cm.intentarReconexionPeriodica()
}

func (cm *ClusterManager) EstadoConexion() string {
	cm.muConexion.Lock()
	defer cm.muConexion.Unlock()

	if cm.cliente != nil {
		return "conectado"
	}
	if cm.reconectando {
		return "reconectando"
	}
	return "desconectado"
}

func (cm *ClusterManager) ObtenerUltimaSincronizacion() time.Time {
	cm.muSincronizando.Lock()
	defer cm.muSincronizando.Unlock()
	return cm.ultimaSincro
}

func (cm *ClusterManager) iniciarListenersConsultas() {
	if cm.cliente == nil {
		return
	}

	topicoRango := fmt.Sprintf("nodo.%s.consulta.rango", cm.nodeID)
	cm.cliente.Suscribir(topicoRango, cm.manejarConsultaRango)

	topicoUltimo := fmt.Sprintf("nodo.%s.consulta.ultimo", cm.nodeID)
	cm.cliente.Suscribir(topicoUltimo, cm.manejarConsultaUltimo)

	topicoPrimero := fmt.Sprintf("nodo.%s.consulta.primero", cm.nodeID)
	cm.cliente.Suscribir(topicoPrimero, cm.manejarConsultaPrimero)

	log.Printf("Listeners de consultas iniciados para nodo %s", cm.nodeID)
}

func (cm *ClusterManager) manejarConsultaRango(topico string, payload interface{}) {
	payloadBytes, ok := payload.([]byte)
	if !ok {
		log.Printf("Error: payload no es []byte en consulta rango")
		return
	}

	var solicitud SolicitudConsulta
	if err := json.Unmarshal(payloadBytes, &solicitud); err != nil {
		log.Printf("Error al deserializar solicitud de rango: %v", err)
		return
	}

	mediciones, err := cm.manager.ConsultarRango(solicitud.Serie, solicitud.TiempoInicio, solicitud.TiempoFin)

	respuesta := RespuestaConsulta{
		Mediciones: mediciones,
	}

	if err != nil {
		respuesta.Error = err.Error()
	}

	respuestaBytes, err := json.Marshal(respuesta)
	if err != nil {
		log.Printf("Error al serializar respuesta de consulta: %v", err)
		return
	}

	cm.cliente.Publicar(topico+".respuesta", respuestaBytes)
}

func (cm *ClusterManager) manejarConsultaUltimo(topico string, payload interface{}) {
	payloadBytes, ok := payload.([]byte)
	if !ok {
		log.Printf("Error: payload no es []byte en consulta ultimo")
		return
	}

	var solicitud map[string]string
	if err := json.Unmarshal(payloadBytes, &solicitud); err != nil {
		log.Printf("Error al deserializar solicitud de último punto: %v", err)
		return
	}

	nombreSerie := solicitud["serie"]
	medicion, err := cm.manager.ConsultarUltimoPunto(nombreSerie)

	var respuestaBytes []byte
	var errMarshal error

	if err != nil {
		respuesta := map[string]string{"error": err.Error()}
		respuestaBytes, errMarshal = json.Marshal(respuesta)
	} else {
		respuestaBytes, errMarshal = json.Marshal(medicion)
	}

	if errMarshal != nil {
		log.Printf("Error al serializar respuesta de último punto: %v", errMarshal)
		return
	}

	cm.cliente.Publicar(topico+".respuesta", respuestaBytes)
}

func (cm *ClusterManager) manejarConsultaPrimero(topico string, payload interface{}) {
	payloadBytes, ok := payload.([]byte)
	if !ok {
		log.Printf("Error: payload no es []byte en consulta primero")
		return
	}

	var solicitud map[string]string
	if err := json.Unmarshal(payloadBytes, &solicitud); err != nil {
		log.Printf("Error al deserializar solicitud de primer punto: %v", err)
		return
	}

	nombreSerie := solicitud["serie"]
	medicion, err := cm.manager.ConsultarPrimerPunto(nombreSerie)

	var respuestaBytes []byte
	var errMarshal error

	if err != nil {
		respuesta := map[string]string{"error": err.Error()}
		respuestaBytes, errMarshal = json.Marshal(respuesta)
	} else {
		respuestaBytes, errMarshal = json.Marshal(medicion)
	}

	if errMarshal != nil {
		log.Printf("Error al serializar respuesta de primer punto: %v", errMarshal)
		return
	}

	cm.cliente.Publicar(topico+".respuesta", respuestaBytes)
}
