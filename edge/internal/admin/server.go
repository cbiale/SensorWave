package admin

import (
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	edgesensorwave "edgesensorwave/pkg"
)

// Config contiene la configuración del servidor admin
type Config struct {
	Host    string
	Port    string
	DevMode bool
}

// Server maneja todas las peticiones del admin web
type Server struct {
	db        *edgesensorwave.DB
	config    *Config
	templates *template.Template
	alertas   *AlertManager
	mutex     sync.RWMutex
}

// Archivos embebidos comentados - go:embed no soporta rutas con ..
// Para usar embed, los archivos deben estar en un subdirectorio del paquete
// //go:embed templates/*.html
// var templateFiles embed.FS

// //go:embed static/*
// var staticFiles embed.FS

// NewServer crea una nueva instancia del servidor admin
func NewServer(db *edgesensorwave.DB, config *Config) *Server {
	server := &Server{
		db:      db,
		config:  config,
		alertas: NewAlertManager(db),
	}
	
	// Cargar templates
	server.loadTemplates()
	
	return server
}

// loadTemplates carga y compila los templates HTML
func (s *Server) loadTemplates() {
	var err error
	
	if s.config.DevMode {
		// En modo desarrollo, cargar desde archivos locales
		s.templates, err = template.ParseGlob("web/templates/*.html")
	} else {
		// En producción, usar archivos del directorio por ahora
		s.templates, err = template.ParseGlob("web/templates/*.html")
	}
	
	if err != nil {
		log.Printf("Warning: Error cargando templates: %v", err)
		// Crear templates básicos como fallback
		s.templates = template.New("fallback")
		s.templates.Parse(`<!DOCTYPE html><html><head><title>EdgeSensorWave Admin</title></head><body><h1>EdgeSensorWave Admin</h1><p>Templates not loaded</p></body></html>`)
	}
}

// RegisterRoutes registra todas las rutas del admin
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// Páginas principales
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/consulta", s.handleQueryBuilder)
	mux.HandleFunc("/alertas", s.handleAlertas)
	mux.HandleFunc("/exportar", s.handleExportar)
	mux.HandleFunc("/mantenimiento", s.handleMantenimiento)
	
	// API endpoints
	mux.HandleFunc("/api/metrics", s.handleAPIMetrics)
	mux.HandleFunc("/api/sensors", s.handleAPISensors)
	mux.HandleFunc("/api/query/execute", s.handleAPIQueryExecute)
	mux.HandleFunc("/api/alerts", s.handleAPIAlertas)
	mux.HandleFunc("/api/export/csv", s.handleAPIExportCSV)
	mux.HandleFunc("/api/export/json", s.handleAPIExportJSON)
	mux.HandleFunc("/api/maintenance/compact", s.handleAPICompact)
	mux.HandleFunc("/api/maintenance/stats", s.handleAPIMaintenanceStats)
	
	// Archivos estáticos
	if s.config.DevMode {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	} else {
		// Usar archivos del directorio web/static por ahora
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	}
}

// Métricas del dashboard
type DashboardMetrics struct {
	TotalSensors     int64     `json:"totalSensors"`
	TotalRecords     int64     `json:"totalRecords"`
	DatabaseSize     string    `json:"databaseSize"`
	LastCompaction   string    `json:"lastCompaction"`
	WritesPerMinute  float64   `json:"writesPerMinute"`
	ReadsPerMinute   float64   `json:"readsPerMinute"`
	MemoryUsage      string    `json:"memoryUsage"`
	UptimeSeconds    int64     `json:"uptimeSeconds"`
	ActiveAlerts     int       `json:"activeAlerts"`
	LastUpdate       time.Time `json:"lastUpdate"`
}

// SensorInfo información de un sensor individual
type SensorInfo struct {
	ID          string    `json:"id"`
	LastValue   float64   `json:"lastValue"`
	LastUpdate  time.Time `json:"lastUpdate"`
	Quality     string    `json:"quality"`
	Status      string    `json:"status"` // online, offline, warning
	RecordCount int64     `json:"recordCount"`
}

// QueryRequest representa una petición de consulta
type QueryRequest struct {
	Pattern   string    `json:"pattern"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Limit     int       `json:"limit"`
	Format    string    `json:"format"` // table, chart
}

// QueryResult resultado de una consulta
type QueryResult struct {
	Columns []string                 `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
	Count   int                      `json:"count"`
	Query   string                   `json:"query"`
}

// AlertRule regla de alerta
type AlertRule struct {
	ID          string  `json:"id"`
	SensorID    string  `json:"sensorId"`
	Operator    string  `json:"operator"` // >, <, ==, !=
	Threshold   float64 `json:"threshold"`
	Enabled     bool    `json:"enabled"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Created     time.Time `json:"created"`
	LastCheck   time.Time `json:"lastCheck"`
}

// AlertEvent evento de alerta disparada
type AlertEvent struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"ruleId"`
	SensorID  string    `json:"sensorId"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Resolved  bool      `json:"resolved"`
}

// ExportRequest petición de exportación
type ExportRequest struct {
	Pattern      string    `json:"pattern"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Format       string    `json:"format"` // csv, json
	IncludeQuality bool    `json:"includeQuality"`
	IncludeMetadata bool   `json:"includeMetadata"`
	Filename     string    `json:"filename"`
}

// MaintenanceStats estadísticas de mantenimiento
type MaintenanceStats struct {
	DatabaseSize     int64     `json:"databaseSize"`
	FragmentationPct float64   `json:"fragmentationPct"`
	LastCompaction   time.Time `json:"lastCompaction"`
	CompactionCount  int64     `json:"compactionCount"`
	CleanupEnabled   bool      `json:"cleanupEnabled"`
	RetentionDays    int       `json:"retentionDays"`
	OldestRecord     time.Time `json:"oldestRecord"`
	NewestRecord     time.Time `json:"newestRecord"`
}

// renderTemplate renderiza un template con datos usando base.html como layout
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Ejecutar base.html que incluirá el template específico via {{template "content" .}}
	err := s.templates.ExecuteTemplate(w, "base.html", data)
	if err != nil {
		log.Printf("Error renderizando template %s con base.html: %v", name, err)
		http.Error(w, "Error interno del servidor", http.StatusInternalServerError)
	}
}

// enableCORS habilita CORS para APIs
func (s *Server) enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// logRequest registra peticiones HTTP
func (s *Server) logRequest(r *http.Request) {
	log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
}