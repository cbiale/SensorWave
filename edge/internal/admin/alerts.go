package admin

import (
	"fmt"
	"sync"
	"time"

	edgesensorwave "edgesensorwave/pkg"
)

// AlertManager gestiona las alertas del sistema
type AlertManager struct {
	db      *edgesensorwave.DB
	rules   map[string]*AlertRule
	events  []AlertEvent
	mutex   sync.RWMutex
	running bool
	stop    chan bool
}

// NewAlertManager crea un nuevo gestor de alertas
func NewAlertManager(db *edgesensorwave.DB) *AlertManager {
	am := &AlertManager{
		db:     db,
		rules:  make(map[string]*AlertRule),
		events: make([]AlertEvent, 0),
		stop:   make(chan bool),
	}
	
	// Iniciar monitoreo en background
	go am.monitor()
	
	return am
}

// AddRule agrega una nueva regla de alerta
func (am *AlertManager) AddRule(rule *AlertRule) error {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	
	if rule.ID == "" {
		rule.ID = generateAlertID()
	}
	
	rule.Created = time.Now()
	rule.LastCheck = time.Time{}
	
	am.rules[rule.ID] = rule
	return nil
}

// GetRules retorna todas las reglas de alerta
func (am *AlertManager) GetRules() []*AlertRule {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	
	rules := make([]*AlertRule, 0, len(am.rules))
	for _, rule := range am.rules {
		rules = append(rules, rule)
	}
	
	return rules
}

// GetActiveAlertsCount retorna el número de alertas activas
func (am *AlertManager) GetActiveAlertsCount() int {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	
	count := 0
	for _, rule := range am.rules {
		if rule.Enabled {
			count++
		}
	}
	
	return count
}

// GetRecentEvents retorna eventos recientes de alertas
func (am *AlertManager) GetRecentEvents(limit int) []AlertEvent {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	
	if len(am.events) <= limit {
		return am.events
	}
	
	return am.events[len(am.events)-limit:]
}

// AddEvent agrega un nuevo evento de alerta
func (am *AlertManager) AddEvent(event AlertEvent) {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	
	if event.ID == "" {
		event.ID = generateEventID()
	}
	
	event.Timestamp = time.Now()
	am.events = append(am.events, event)
	
	// Mantener solo los últimos 1000 eventos
	if len(am.events) > 1000 {
		am.events = am.events[len(am.events)-1000:]
	}
}

// monitor ejecuta el monitoreo continuo de alertas
func (am *AlertManager) monitor() {
	am.running = true
	ticker := time.NewTicker(30 * time.Second) // Chequear cada 30 segundos
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			am.checkAlerts()
		case <-am.stop:
			am.running = false
			return
		}
	}
}

// checkAlerts verifica todas las reglas de alerta activas
func (am *AlertManager) checkAlerts() {
	am.mutex.RLock()
	rules := make([]*AlertRule, 0, len(am.rules))
	for _, rule := range am.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	am.mutex.RUnlock()
	
	for _, rule := range rules {
		am.checkRule(rule)
	}
}

// checkRule verifica una regla específica
func (am *AlertManager) checkRule(rule *AlertRule) {
	// Obtener último valor del sensor
	clave, valor, err := am.db.BuscarUltimo(rule.SensorID)
	if err != nil || clave == nil || valor == nil {
		return
	}
	
	// Actualizar timestamp de último chequeo
	am.mutex.Lock()
	rule.LastCheck = time.Now()
	am.mutex.Unlock()
	
	// Evaluar condición
	triggered := false
	switch rule.Operator {
	case ">":
		triggered = valor.Valor > rule.Threshold
	case "<":
		triggered = valor.Valor < rule.Threshold
	case "==":
		triggered = valor.Valor == rule.Threshold
	case "!=":
		triggered = valor.Valor != rule.Threshold
	case ">=":
		triggered = valor.Valor >= rule.Threshold
	case "<=":
		triggered = valor.Valor <= rule.Threshold
	}
	
	if triggered {
		message := fmt.Sprintf("Alerta '%s': Sensor %s tiene valor %.2f %s %.2f",
			rule.Name, rule.SensorID, valor.Valor, rule.Operator, rule.Threshold)
		
		event := AlertEvent{
			RuleID:    rule.ID,
			SensorID:  rule.SensorID,
			Value:     valor.Valor,
			Threshold: rule.Threshold,
			Message:   message,
			Resolved:  false,
		}
		
		am.AddEvent(event)
	}
}

// Stop detiene el monitoreo de alertas
func (am *AlertManager) Stop() {
	if am.running {
		am.stop <- true
	}
}

// DeleteRule elimina una regla de alerta
func (am *AlertManager) DeleteRule(ruleID string) bool {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	
	if _, exists := am.rules[ruleID]; exists {
		delete(am.rules, ruleID)
		return true
	}
	
	return false
}

// ToggleRule habilita/deshabilita una regla
func (am *AlertManager) ToggleRule(ruleID string) bool {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	
	if rule, exists := am.rules[ruleID]; exists {
		rule.Enabled = !rule.Enabled
		return true
	}
	
	return false
}

// UpdateRule actualiza una regla existente
func (am *AlertManager) UpdateRule(rule *AlertRule) error {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	
	if _, exists := am.rules[rule.ID]; !exists {
		return fmt.Errorf("regla no encontrada: %s", rule.ID)
	}
	
	am.rules[rule.ID] = rule
	return nil
}

// generateAlertID genera un ID único para una regla de alerta
func generateAlertID() string {
	return fmt.Sprintf("alert_%d", time.Now().UnixNano())
}

// generateEventID genera un ID único para un evento de alerta
func generateEventID() string {
	return fmt.Sprintf("event_%d", time.Now().UnixNano())
}

// GetRuleByID obtiene una regla por su ID
func (am *AlertManager) GetRuleByID(id string) (*AlertRule, bool) {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	
	rule, exists := am.rules[id]
	return rule, exists
}

// GetEventsByRule obtiene eventos de una regla específica
func (am *AlertManager) GetEventsByRule(ruleID string) []AlertEvent {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	
	var events []AlertEvent
	for _, event := range am.events {
		if event.RuleID == ruleID {
			events = append(events, event)
		}
	}
	
	return events
}

// ResolveEvent marca un evento como resuelto
func (am *AlertManager) ResolveEvent(eventID string) bool {
	am.mutex.Lock()
	defer am.mutex.Unlock()
	
	for i := range am.events {
		if am.events[i].ID == eventID {
			am.events[i].Resolved = true
			return true
		}
	}
	
	return false
}

// GetAlertStats retorna estadísticas de alertas
func (am *AlertManager) GetAlertStats() map[string]interface{} {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	
	totalRules := len(am.rules)
	activeRules := 0
	totalEvents := len(am.events)
	unresolvedEvents := 0
	
	for _, rule := range am.rules {
		if rule.Enabled {
			activeRules++
		}
	}
	
	for _, event := range am.events {
		if !event.Resolved {
			unresolvedEvents++
		}
	}
	
	return map[string]interface{}{
		"totalRules":       totalRules,
		"activeRules":      activeRules,
		"totalEvents":      totalEvents,
		"unresolvedEvents": unresolvedEvents,
		"running":          am.running,
	}
}