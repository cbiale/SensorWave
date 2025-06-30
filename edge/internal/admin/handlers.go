package admin

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

var startTime = time.Now()

// handleDashboard maneja la página principal del dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	// Obtener métricas básicas
	metrics, err := s.getDashboardMetrics()
	if err != nil {
		log.Printf("Error obteniendo métricas: %v", err)
		http.Error(w, "Error obteniendo métricas", http.StatusInternalServerError)
		return
	}
	
	// Obtener sensores activos
	sensors, err := s.getActiveSensors()
	if err != nil {
		log.Printf("Error obteniendo sensores: %v", err)
		sensors = []SensorInfo{} // Lista vacía en caso de error
	}
	
	data := map[string]interface{}{
		"Title":   "Dashboard - EdgeSensorWave Admin",
		"Metrics": metrics,
		"Sensors": sensors,
		"Version": "1.0.5",
		"Page":    "dashboard",
	}
	
	s.renderTemplate(w, "dashboard.html", data)
}

// handleQueryBuilder maneja la página del constructor de consultas
func (s *Server) handleQueryBuilder(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	// Obtener lista de sensores para autocompletado
	sensors, err := s.db.ListarSensores("*")
	if err != nil {
		log.Printf("Error obteniendo sensores: %v", err)
		sensors = []string{}
	}
	
	data := map[string]interface{}{
		"Title":   "Query Builder - EdgeSensorWave Admin",
		"Sensors": sensors,
		"Version": "1.0.5",
		"Page":    "query",
	}
	
	s.renderTemplate(w, "query.html", data)
}

// handleAlertas maneja la página de gestión de alertas
func (s *Server) handleAlertas(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	
	switch r.Method {
	case http.MethodGet:
		alertas := s.alertas.GetRules()
		eventos := s.alertas.GetRecentEvents(50)
		
		data := map[string]interface{}{
			"Title":   "Alertas - EdgeSensorWave Admin",
			"Rules":   alertas,
			"Events":  eventos,
			"Version": "1.0.5",
			"Page":    "alerts",
		}
		
		s.renderTemplate(w, "alerts.html", data)
		
	case http.MethodPost:
		s.handleCreateAlert(w, r)
		
	default:
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	}
}

// handleExportar maneja la página de exportación
func (s *Server) handleExportar(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	// Obtener lista de sensores
	sensors, err := s.db.ListarSensores("*")
	if err != nil {
		sensors = []string{}
	}
	
	data := map[string]interface{}{
		"Title":   "Exportar Datos - EdgeSensorWave Admin",
		"Sensors": sensors,
		"Version": "1.0.5",
		"Page":    "export",
	}
	
	s.renderTemplate(w, "export.html", data)
}

// handleMantenimiento maneja la página de mantenimiento
func (s *Server) handleMantenimiento(w http.ResponseWriter, r *http.Request) {
	s.logRequest(r)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	// Obtener estadísticas de mantenimiento
	stats, err := s.getMaintenanceStats()
	if err != nil {
		log.Printf("Error obteniendo stats de mantenimiento: %v", err)
		stats = &MaintenanceStats{}
	}
	
	data := map[string]interface{}{
		"Title":   "Mantenimiento - EdgeSensorWave Admin",
		"Stats":   stats,
		"Version": "1.0.5",
		"Page":    "maintenance",
	}
	
	s.renderTemplate(w, "maintenance.html", data)
}

// API HANDLERS

// handleAPIMetrics retorna métricas del sistema en JSON para dashboard
func (s *Server) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	metrics, err := s.getDashboardMetrics()
	if err != nil {
		http.Error(w, "Error obteniendo métricas", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleAPISensors retorna información de sensores activos
func (s *Server) handleAPISensors(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	sensors, err := s.getActiveSensors()
	if err != nil {
		http.Error(w, "Error obteniendo sensores", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sensors)
}

// handleAPIQueryExecute ejecuta una consulta y retorna resultados
func (s *Server) handleAPIQueryExecute(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	var req QueryRequest
	
	// Verificar content type y parsear apropiadamente
	contentType := r.Header.Get("Content-Type")
	if contentType == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
	} else {
		// Parsear como formulario HTML
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parseando formulario", http.StatusBadRequest)
			return
		}
		
		req.Pattern = r.FormValue("pattern")
		req.Limit = 1000 // Default
		if limitStr := r.FormValue("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil {
				req.Limit = limit
			}
		}
		
		// Parsear fechas
		if startTimeStr := r.FormValue("startTime"); startTimeStr != "" {
			if startTime, err := time.Parse("2006-01-02T15:04", startTimeStr); err == nil {
				req.StartTime = startTime
			}
		}
		if endTimeStr := r.FormValue("endTime"); endTimeStr != "" {
			if endTime, err := time.Parse("2006-01-02T15:04", endTimeStr); err == nil {
				req.EndTime = endTime
			}
		}
		
		req.Format = r.FormValue("format")
		if req.Format == "" {
			req.Format = "table"
		}
	}
	
	// Ejecutar consulta
	result, err := s.executeQuery(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error ejecutando consulta: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Si es JSON, responder con JSON
	if contentType == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}
	
	// Si es formulario HTML, responder con HTML para htmx
	w.Header().Set("Content-Type", "text/html")
	s.renderQueryResults(w, result)
}

// handleAPIAlertas maneja las operaciones de alertas
func (s *Server) handleAPIAlertas(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	switch r.Method {
	case http.MethodGet:
		rules := s.alertas.GetRules()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rules)
		
	case http.MethodPost:
		var rule AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "JSON inválido", http.StatusBadRequest)
			return
		}
		
		if err := s.alertas.AddRule(&rule); err != nil {
			http.Error(w, fmt.Sprintf("Error creando alerta: %v", err), http.StatusInternalServerError)
			return
		}
		
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)
		
	default:
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	}
}

// handleAPIExportCSV exporta datos en formato CSV
func (s *Server) handleAPIExportCSV(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	
	// Configurar headers para descarga
	filename := req.Filename
	if filename == "" {
		filename = fmt.Sprintf("export_%s.csv", time.Now().Format("20060102_150405"))
	}
	
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	
	// Crear writer CSV
	writer := csv.NewWriter(w)
	defer writer.Flush()
	
	// Escribir headers
	headers := []string{"sensor_id", "timestamp", "value"}
	if req.IncludeQuality {
		headers = append(headers, "quality")
	}
	if req.IncludeMetadata {
		headers = append(headers, "metadata")
	}
	writer.Write(headers)
	
	// Exportar datos
	err := s.exportData(&req, writer)
	if err != nil {
		log.Printf("Error exportando: %v", err)
		// Note: No podemos enviar error HTTP aquí ya que ya empezamos a escribir la respuesta
	}
}

// handleAPIExportJSON exporta datos en formato JSON
func (s *Server) handleAPIExportJSON(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	
	// Configurar headers
	filename := req.Filename
	if filename == "" {
		filename = fmt.Sprintf("export_%s.json", time.Now().Format("20060102_150405"))
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	
	// Exportar datos como JSON
	data, err := s.exportDataAsJSON(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error exportando: %v", err), http.StatusInternalServerError)
		return
	}
	
	json.NewEncoder(w).Encode(data)
}

// handleAPICompact ejecuta compactación de la base de datos
func (s *Server) handleAPICompact(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	start := time.Now()
	err := s.db.Compactar()
	duration := time.Since(start)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Error compactando: %v", err), http.StatusInternalServerError)
		return
	}
	
	response := map[string]interface{}{
		"success":     true,
		"duration_ms": duration.Milliseconds(),
		"message":     "Compactación completada exitosamente",
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleAPIMaintenanceStats retorna estadísticas de mantenimiento
func (s *Server) handleAPIMaintenanceStats(w http.ResponseWriter, r *http.Request) {
	s.enableCORS(w)
	
	if r.Method != http.MethodGet {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}
	
	stats, err := s.getMaintenanceStats()
	if err != nil {
		http.Error(w, "Error obteniendo estadísticas", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HELPER METHODS

// getDashboardMetrics obtiene métricas para el dashboard
func (s *Server) getDashboardMetrics() (*DashboardMetrics, error) {
	stats, err := s.db.Estadisticas()
	if err != nil {
		return nil, err
	}
	
	// Obtener métricas de memoria
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryUsage := fmt.Sprintf("%.2f MB", float64(m.Alloc)/1024/1024)
	
	return &DashboardMetrics{
		TotalSensors:     stats.NumSensores,
		TotalRecords:     stats.NumRegistros,
		DatabaseSize:     formatBytes(stats.TamañoBytes),
		LastCompaction:   stats.UltimaCompactacion.Format("2006-01-02 15:04:05"),
		WritesPerMinute:  calculateWritesPerMinute(), // Placeholder
		ReadsPerMinute:   calculateReadsPerMinute(),  // Placeholder
		MemoryUsage:      memoryUsage,
		UptimeSeconds:    int64(time.Since(startTime).Seconds()),
		ActiveAlerts:     s.alertas.GetActiveAlertsCount(),
		LastUpdate:       time.Now(),
	}, nil
}

// getActiveSensors obtiene información de sensores activos
func (s *Server) getActiveSensors() ([]SensorInfo, error) {
	sensors, err := s.db.ListarSensores("*")
	if err != nil {
		return nil, err
	}
	
	var sensorInfos []SensorInfo
	
	for _, sensorID := range sensors {
		// Obtener último valor
		clave, valor, err := s.db.BuscarUltimo(sensorID)
		if err != nil {
			continue
		}
		
		if clave == nil || valor == nil {
			continue
		}
		
		// Determinar estado basado en tiempo desde última actualización
		status := "online"
		timeSinceUpdate := time.Since(clave.Timestamp)
		if timeSinceUpdate > 5*time.Minute {
			status = "offline"
		} else if timeSinceUpdate > time.Minute {
			status = "warning"
		}
		
		sensorInfo := SensorInfo{
			ID:          sensorID,
			LastValue:   valor.Valor,
			LastUpdate:  clave.Timestamp,
			Quality:     valor.Calidad.String(),
			Status:      status,
			RecordCount: 0, // Placeholder - podría ser costoso calcularlo aquí
		}
		
		sensorInfos = append(sensorInfos, sensorInfo)
	}
	
	return sensorInfos, nil
}

// executeQuery ejecuta una consulta y devuelve resultados formateados
func (s *Server) executeQuery(req *QueryRequest) (*QueryResult, error) {
	// Limitar resultados si no se especifica límite
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	
	iter, err := s.db.ConsultarRangoConLimite(req.Pattern, req.StartTime, req.EndTime, limit)
	if err != nil {
		return nil, err
	}
	defer iter.Cerrar()
	
	columns := []string{"sensor_id", "timestamp", "value", "quality"}
	var rows []map[string]interface{}
	count := 0
	
	for iter.Siguiente() {
		clave := iter.Clave()
		valor := iter.Valor()
		
		row := map[string]interface{}{
			"sensor_id": clave.IDSensor,
			"timestamp": clave.Timestamp.Format("2006-01-02 15:04:05"),
			"value":     valor.Valor,
			"quality":   valor.Calidad.String(),
		}
		
		rows = append(rows, row)
		count++
	}
	
	queryStr := fmt.Sprintf(`db.ConsultarRangoConLimite("%s", time.Parse("%s"), time.Parse("%s"), %d)`,
		req.Pattern, req.StartTime.Format("2006-01-02 15:04:05"), 
		req.EndTime.Format("2006-01-02 15:04:05"), limit)
	
	return &QueryResult{
		Columns: columns,
		Rows:    rows,
		Count:   count,
		Query:   queryStr,
	}, nil
}

// Funciones helper
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

func calculateWritesPerMinute() float64 {
	// Placeholder - implementar lógica real de métricas
	return 850.5
}

func calculateReadsPerMinute() float64 {
	// Placeholder - implementar lógica real de métricas
	return 1250.3
}

// renderQueryResults renderiza los resultados de consulta como HTML
func (s *Server) renderQueryResults(w http.ResponseWriter, result *QueryResult) {
	if result == nil || len(result.Rows) == 0 {
		fmt.Fprint(w, `
			<div class="empty-state">
				<div class="empty-icon">📭</div>
				<h4>Sin resultados</h4>
				<p>La consulta no devolvió ningún resultado</p>
			</div>
		`)
		return
	}
	
	fmt.Fprintf(w, `
		<div class="results-summary">
			<span>📊 %d resultados encontrados</span>
			<span>⚡ Consulta: %s</span>
		</div>
		<div class="table-container">
			<table class="results-table">
				<thead>
					<tr>
	`, result.Count, result.Query)
	
	// Headers
	for _, col := range result.Columns {
		fmt.Fprintf(w, "<th>%s</th>", col)
	}
	fmt.Fprint(w, "</tr></thead><tbody>")
	
	// Rows
	for _, row := range result.Rows {
		fmt.Fprint(w, "<tr>")
		for _, col := range result.Columns {
			value := row[col]
			if col == "value" {
				if v, ok := value.(float64); ok {
					fmt.Fprintf(w, "<td>%.2f</td>", v)
				} else {
					fmt.Fprintf(w, "<td>%v</td>", value)
				}
			} else {
				fmt.Fprintf(w, "<td>%v</td>", value)
			}
		}
		fmt.Fprint(w, "</tr>")
	}
	
	fmt.Fprint(w, "</tbody></table></div>")
}