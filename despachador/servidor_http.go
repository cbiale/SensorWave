package despachador

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cbiale/sensorwave/tipos"
)

// ServidorHTTP expone la API REST del despachador
type ServidorHTTP struct {
	manager *ManagerDespachador
	server  *http.Server
}

// NuevoServidorHTTP crea un nuevo servidor HTTP para el despachador
func NuevoServidorHTTP(manager *ManagerDespachador, puerto string) *ServidorHTTP {
	s := &ServidorHTTP{
		manager: manager,
	}

	mux := http.NewServeMux()

	// Endpoints de información
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/nodos", s.handleListarNodos)
	mux.HandleFunc("GET /api/series", s.handleListarSeries)
	mux.HandleFunc("GET /api/series/{path...}", s.handleObtenerSerie)

	// Endpoints de consulta
	mux.HandleFunc("POST /api/consulta/rango", s.handleConsultarRango)
	mux.HandleFunc("POST /api/consulta/ultimo", s.handleConsultarUltimo)
	mux.HandleFunc("POST /api/consulta/agregacion", s.handleConsultarAgregacion)
	mux.HandleFunc("POST /api/consulta/agregacion-temporal", s.handleConsultarAgregacionTemporal)

	s.server = &http.Server{
		Addr:         ":" + puerto,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return s
}

// Iniciar inicia el servidor HTTP en segundo plano
func (s *ServidorHTTP) Iniciar() error {
	log.Printf("Servidor HTTP del despachador escuchando en %s", s.server.Addr)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Error en servidor HTTP: %v", err)
		}
	}()
	return nil
}

// Detener detiene el servidor HTTP
func (s *ServidorHTTP) Detener() error {
	log.Printf("Deteniendo servidor HTTP del despachador...")
	return s.server.Close()
}

// ============================================================================
// HANDLERS DE INFORMACIÓN
// ============================================================================

// StatusResponse respuesta del endpoint /api/status
type StatusResponse struct {
	NumNodos  int `json:"num_nodos"`
	NumSeries int `json:"num_series"`
}

func (s *ServidorHTTP) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := s.manager.ObtenerEstadisticas()
	respuesta := StatusResponse{
		NumNodos:  stats.NumNodos,
		NumSeries: stats.NumSeries,
	}
	s.enviarJSON(w, respuesta)
}

func (s *ServidorHTTP) handleListarNodos(w http.ResponseWriter, r *http.Request) {
	nodos := s.manager.ListarNodos()
	s.enviarJSON(w, nodos)
}

// SerieResponse respuesta con información de serie para JSON
type SerieResponse struct {
	Path                 string            `json:"path"`
	NodoID               string            `json:"nodo_id"`
	TipoDatos            string            `json:"tipo_datos"`
	Tags                 map[string]string `json:"tags,omitempty"`
	TamanoBloque         int               `json:"tamano_bloque"`
	TiempoAlmacenamiento int64             `json:"tiempo_almacenamiento"`
	CompresionBytes      string            `json:"compresion_bytes"`
	CompresionBloque     string            `json:"compresion_bloque"`
}

func serieToResponse(si SerieInfo) SerieResponse {
	return SerieResponse{
		Path:                 si.Path,
		NodoID:               si.NodoID,
		TipoDatos:            si.TipoDatos.String(),
		Tags:                 si.Tags,
		TamanoBloque:         si.TamañoBloque,
		TiempoAlmacenamiento: si.TiempoAlmacenamiento,
		CompresionBytes:      string(si.CompresionBytes),
		CompresionBloque:     string(si.CompresionBloque),
	}
}

func (s *ServidorHTTP) handleListarSeries(w http.ResponseWriter, r *http.Request) {
	patron := r.URL.Query().Get("patron")
	if patron == "" {
		patron = "*"
	}

	series := s.manager.ListarSeries(patron)

	// Convertir a formato JSON
	respuesta := make([]SerieResponse, len(series))
	for i, si := range series {
		respuesta[i] = serieToResponse(si)
	}

	s.enviarJSON(w, respuesta)
}

func (s *ServidorHTTP) handleObtenerSerie(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	if path == "" {
		s.enviarError(w, http.StatusBadRequest, "path de serie requerido")
		return
	}

	serie := s.manager.ObtenerSerie(path)
	if serie == nil {
		s.enviarError(w, http.StatusNotFound, fmt.Sprintf("serie '%s' no encontrada", path))
		return
	}

	s.enviarJSON(w, serieToResponse(*serie))
}

// ============================================================================
// HANDLERS DE CONSULTA
// ============================================================================

// ConsultaRangoRequest solicitud de consulta por rango
type ConsultaRangoRequest struct {
	Serie        string `json:"serie"`
	TiempoInicio int64  `json:"tiempo_inicio"` // Unix nanosegundos
	TiempoFin    int64  `json:"tiempo_fin"`    // Unix nanosegundos
}

// ConsultaRangoResponse respuesta de consulta por rango
type ConsultaRangoResponse struct {
	Series             []string        `json:"series"`
	Tiempos            []int64         `json:"tiempos"`
	Valores            [][]interface{} `json:"valores"`
	NodosNoDisponibles []string        `json:"nodos_no_disponibles,omitempty"`
}

func (s *ServidorHTTP) handleConsultarRango(w http.ResponseWriter, r *http.Request) {
	var req ConsultaRangoRequest
	if err := s.leerJSON(r, &req); err != nil {
		s.enviarError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Serie == "" {
		s.enviarError(w, http.StatusBadRequest, "serie requerida")
		return
	}

	tiempoInicio := time.Unix(0, req.TiempoInicio)
	tiempoFin := time.Unix(0, req.TiempoFin)

	resultado, err := s.manager.ConsultarRango(req.Serie, tiempoInicio, tiempoFin)
	if err != nil {
		s.enviarError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respuesta := ConsultaRangoResponse{
		Series:             resultado.Series,
		Tiempos:            resultado.Tiempos,
		Valores:            resultado.Valores,
		NodosNoDisponibles: resultado.NodosNoDisponibles,
	}

	s.enviarJSON(w, respuesta)
}

// ConsultaUltimoRequest solicitud de consulta de último punto
type ConsultaUltimoRequest struct {
	Serie        string `json:"serie"`
	TiempoInicio *int64 `json:"tiempo_inicio,omitempty"` // Unix nanosegundos, opcional
	TiempoFin    *int64 `json:"tiempo_fin,omitempty"`    // Unix nanosegundos, opcional
}

// ConsultaUltimoResponse respuesta de consulta de último punto
type ConsultaUltimoResponse struct {
	Series             []string      `json:"series"`
	Tiempos            []int64       `json:"tiempos"`
	Valores            []interface{} `json:"valores"`
	NodosNoDisponibles []string      `json:"nodos_no_disponibles,omitempty"`
}

func (s *ServidorHTTP) handleConsultarUltimo(w http.ResponseWriter, r *http.Request) {
	var req ConsultaUltimoRequest
	if err := s.leerJSON(r, &req); err != nil {
		s.enviarError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Serie == "" {
		s.enviarError(w, http.StatusBadRequest, "serie requerida")
		return
	}

	var tiempoInicio, tiempoFin *time.Time
	if req.TiempoInicio != nil {
		t := time.Unix(0, *req.TiempoInicio)
		tiempoInicio = &t
	}
	if req.TiempoFin != nil {
		t := time.Unix(0, *req.TiempoFin)
		tiempoFin = &t
	}

	resultado, err := s.manager.ConsultarUltimoPunto(req.Serie, tiempoInicio, tiempoFin)
	if err != nil {
		s.enviarError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respuesta := ConsultaUltimoResponse{
		Series:             resultado.Series,
		Tiempos:            resultado.Tiempos,
		Valores:            resultado.Valores,
		NodosNoDisponibles: resultado.NodosNoDisponibles,
	}

	s.enviarJSON(w, respuesta)
}

// ConsultaAgregacionRequest solicitud de consulta de agregación
type ConsultaAgregacionRequest struct {
	Serie        string   `json:"serie"`
	TiempoInicio int64    `json:"tiempo_inicio"` // Unix nanosegundos
	TiempoFin    int64    `json:"tiempo_fin"`    // Unix nanosegundos
	Agregaciones []string `json:"agregaciones"`  // "promedio", "maximo", "minimo", "suma", "count"
}

// ConsultaAgregacionResponse respuesta de consulta de agregación
type ConsultaAgregacionResponse struct {
	Series             []string    `json:"series"`
	Agregaciones       []string    `json:"agregaciones"`
	Valores            [][]float64 `json:"valores"` // [agregacion][serie]
	NodosNoDisponibles []string    `json:"nodos_no_disponibles,omitempty"`
}

func (s *ServidorHTTP) handleConsultarAgregacion(w http.ResponseWriter, r *http.Request) {
	var req ConsultaAgregacionRequest
	if err := s.leerJSON(r, &req); err != nil {
		s.enviarError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Serie == "" {
		s.enviarError(w, http.StatusBadRequest, "serie requerida")
		return
	}
	if len(req.Agregaciones) == 0 {
		s.enviarError(w, http.StatusBadRequest, "debe especificar al menos una agregación")
		return
	}

	// Convertir strings a TipoAgregacion
	agregaciones := make([]tipos.TipoAgregacion, len(req.Agregaciones))
	for i, a := range req.Agregaciones {
		agregaciones[i] = tipos.TipoAgregacion(a)
	}

	tiempoInicio := time.Unix(0, req.TiempoInicio)
	tiempoFin := time.Unix(0, req.TiempoFin)

	resultado, err := s.manager.ConsultarAgregacion(req.Serie, tiempoInicio, tiempoFin, agregaciones)
	if err != nil {
		s.enviarError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convertir TipoAgregacion a strings
	agregacionesStr := make([]string, len(resultado.Agregaciones))
	for i, a := range resultado.Agregaciones {
		agregacionesStr[i] = string(a)
	}

	respuesta := ConsultaAgregacionResponse{
		Series:             resultado.Series,
		Agregaciones:       agregacionesStr,
		Valores:            resultado.Valores,
		NodosNoDisponibles: resultado.NodosNoDisponibles,
	}

	s.enviarJSON(w, respuesta)
}

// ConsultaAgregacionTemporalRequest solicitud de consulta de agregación temporal
type ConsultaAgregacionTemporalRequest struct {
	Serie        string   `json:"serie"`
	TiempoInicio int64    `json:"tiempo_inicio"` // Unix nanosegundos
	TiempoFin    int64    `json:"tiempo_fin"`    // Unix nanosegundos
	Agregaciones []string `json:"agregaciones"`  // "promedio", "maximo", "minimo", "suma", "count"
	Intervalo    int64    `json:"intervalo"`     // Duration en nanosegundos
}

// ConsultaAgregacionTemporalResponse respuesta de consulta de agregación temporal
type ConsultaAgregacionTemporalResponse struct {
	Series             []string      `json:"series"`
	Tiempos            []int64       `json:"tiempos"`
	Agregaciones       []string      `json:"agregaciones"`
	Valores            [][][]float64 `json:"valores"` // [agregacion][bucket][serie]
	NodosNoDisponibles []string      `json:"nodos_no_disponibles,omitempty"`
}

func (s *ServidorHTTP) handleConsultarAgregacionTemporal(w http.ResponseWriter, r *http.Request) {
	var req ConsultaAgregacionTemporalRequest
	if err := s.leerJSON(r, &req); err != nil {
		s.enviarError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Serie == "" {
		s.enviarError(w, http.StatusBadRequest, "serie requerida")
		return
	}
	if len(req.Agregaciones) == 0 {
		s.enviarError(w, http.StatusBadRequest, "debe especificar al menos una agregación")
		return
	}
	if req.Intervalo <= 0 {
		s.enviarError(w, http.StatusBadRequest, "intervalo debe ser mayor a cero")
		return
	}

	// Convertir strings a TipoAgregacion
	agregaciones := make([]tipos.TipoAgregacion, len(req.Agregaciones))
	for i, a := range req.Agregaciones {
		agregaciones[i] = tipos.TipoAgregacion(a)
	}

	tiempoInicio := time.Unix(0, req.TiempoInicio)
	tiempoFin := time.Unix(0, req.TiempoFin)
	intervalo := time.Duration(req.Intervalo)

	resultado, err := s.manager.ConsultarAgregacionTemporal(req.Serie, tiempoInicio, tiempoFin, agregaciones, intervalo)
	if err != nil {
		s.enviarError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convertir TipoAgregacion a strings
	agregacionesStr := make([]string, len(resultado.Agregaciones))
	for i, a := range resultado.Agregaciones {
		agregacionesStr[i] = string(a)
	}

	respuesta := ConsultaAgregacionTemporalResponse{
		Series:             resultado.Series,
		Tiempos:            resultado.Tiempos,
		Agregaciones:       agregacionesStr,
		Valores:            resultado.Valores,
		NodosNoDisponibles: resultado.NodosNoDisponibles,
	}

	s.enviarJSON(w, respuesta)
}

// ============================================================================
// HELPERS
// ============================================================================

func (s *ServidorHTTP) enviarJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encodificando JSON: %v", err)
	}
}

func (s *ServidorHTTP) enviarError(w http.ResponseWriter, codigo int, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(map[string]string{"error": mensaje})
}

func (s *ServidorHTTP) leerJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error leyendo body: %v", err)
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("error parseando JSON: %v", err)
	}
	return nil
}
