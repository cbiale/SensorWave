package admin

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// exportData exporta datos a un writer CSV
func (s *Server) exportData(req *ExportRequest, writer *csv.Writer) error {
	// Obtener datos usando la consulta
	iter, err := s.db.ConsultarRango(req.Pattern, req.StartTime, req.EndTime)
	if err != nil {
		return fmt.Errorf("error ejecutando consulta: %w", err)
	}
	defer iter.Cerrar()
	
	recordCount := 0
	for iter.Siguiente() {
		clave := iter.Clave()
		valor := iter.Valor()
		
		// Preparar fila básica
		row := []string{
			clave.IDSensor,
			clave.Timestamp.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%.6f", valor.Valor),
		}
		
		// Agregar calidad si se solicita
		if req.IncludeQuality {
			row = append(row, valor.Calidad.String())
		}
		
		// Agregar metadatos si se solicita
		if req.IncludeMetadata {
			metadataStr := ""
			if valor.Metadatos != nil && len(valor.Metadatos) > 0 {
				metadataBytes, _ := json.Marshal(valor.Metadatos)
				metadataStr = string(metadataBytes)
			}
			row = append(row, metadataStr)
		}
		
		// Escribir fila
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("error escribiendo fila: %w", err)
		}
		
		recordCount++
		
		// Flush periódicamente para exportaciones grandes
		if recordCount%1000 == 0 {
			writer.Flush()
		}
	}
	
	return nil
}

// exportDataAsJSON exporta datos como estructura JSON
func (s *Server) exportDataAsJSON(req *ExportRequest) (interface{}, error) {
	// Obtener datos usando la consulta
	iter, err := s.db.ConsultarRango(req.Pattern, req.StartTime, req.EndTime)
	if err != nil {
		return nil, fmt.Errorf("error ejecutando consulta: %w", err)
	}
	defer iter.Cerrar()
	
	var records []map[string]interface{}
	
	for iter.Siguiente() {
		clave := iter.Clave()
		valor := iter.Valor()
		
		record := map[string]interface{}{
			"sensor_id": clave.IDSensor,
			"timestamp": clave.Timestamp.Format("2006-01-02T15:04:05Z"),
			"value":     valor.Valor,
		}
		
		// Agregar calidad si se solicita
		if req.IncludeQuality {
			record["quality"] = valor.Calidad.String()
		}
		
		// Agregar metadatos si se solicita
		if req.IncludeMetadata && valor.Metadatos != nil {
			record["metadata"] = valor.Metadatos
		}
		
		records = append(records, record)
	}
	
	// Estructura final del JSON
	export := map[string]interface{}{
		"export_info": map[string]interface{}{
			"pattern":        req.Pattern,
			"start_time":     req.StartTime.Format("2006-01-02T15:04:05Z"),
			"end_time":       req.EndTime.Format("2006-01-02T15:04:05Z"),
			"generated_at":   time.Now().Format("2006-01-02T15:04:05Z"),
			"total_records":  len(records),
			"include_quality": req.IncludeQuality,
			"include_metadata": req.IncludeMetadata,
		},
		"data": records,
	}
	
	return export, nil
}

// handleCreateAlert maneja la creación de nuevas alertas vía POST
func (s *Server) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}
	
	// Validar regla
	if rule.SensorID == "" {
		http.Error(w, "sensor_id es requerido", http.StatusBadRequest)
		return
	}
	
	if rule.Operator == "" {
		http.Error(w, "operator es requerido", http.StatusBadRequest)
		return
	}
	
	// Agregar regla
	if err := s.alertas.AddRule(&rule); err != nil {
		http.Error(w, fmt.Sprintf("Error creando alerta: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Redireccionar de vuelta a la página de alertas
	http.Redirect(w, r, "/alertas", http.StatusSeeOther)
}

// getMaintenanceStats obtiene estadísticas de mantenimiento
func (s *Server) getMaintenanceStats() (*MaintenanceStats, error) {
	stats, err := s.db.Estadisticas()
	if err != nil {
		return nil, err
	}
	
	// Calcular fragmentación (placeholder)
	fragmentationPct := 15.5 // Esto sería calculado basado en las métricas reales de Pebble
	
	// Buscar registro más antiguo y más nuevo
	oldestRecord := time.Now()
	newestRecord := time.Time{}
	
	// Obtener algunos sensores para determinar rango temporal
	sensors, err := s.db.ListarSensores("*")
	if err == nil && len(sensors) > 0 {
		// Intentar obtener primer y último registro de algunos sensores
		for i, sensor := range sensors {
			if i > 5 { // Limitar a primeros 5 sensores para eficiencia
				break
			}
			
			// Buscar primer registro
			inicio := time.Time{}
			fin := time.Now()
			
			iter, err := s.db.ConsultarRangoConLimite(sensor, inicio, fin, 1)
			if err != nil {
				continue
			}
			
			if iter.Siguiente() {
				clave := iter.Clave()
				if clave.Timestamp.Before(oldestRecord) {
					oldestRecord = clave.Timestamp
				}
			}
			iter.Cerrar()
			
			// Buscar último registro
			ultimaClave, _, err := s.db.BuscarUltimo(sensor)
			if err == nil && ultimaClave != nil {
				if ultimaClave.Timestamp.After(newestRecord) {
					newestRecord = ultimaClave.Timestamp
				}
			}
		}
	}
	
	return &MaintenanceStats{
		DatabaseSize:     stats.TamañoBytes,
		FragmentationPct: fragmentationPct,
		LastCompaction:   stats.UltimaCompactacion,
		CompactionCount:  42, // Placeholder
		CleanupEnabled:   true,
		RetentionDays:    30,
		OldestRecord:     oldestRecord,
		NewestRecord:     newestRecord,
	}, nil
}