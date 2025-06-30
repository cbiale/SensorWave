package edgesensorwave

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// ValidarIDSensor valida que un ID de sensor cumpla con las reglas de nomenclatura
func ValidarIDSensor(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("ID de sensor vacío")
	}
	
	if len(id) > 255 {
		return fmt.Errorf("ID de sensor demasiado largo: %d caracteres (máximo 255)", len(id))
	}
	
	// Verificar que comience con letra o número
	if !unicode.IsLetter(rune(id[0])) && !unicode.IsDigit(rune(id[0])) {
		return fmt.Errorf("ID de sensor debe comenzar con letra o número")
	}
	
	// Verificar caracteres válidos (letras, números, punto, guión bajo, guión)
	for i, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || 
			 (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("carácter inválido en posición %d: '%c' (solo se permiten letras, números, '.', '_', '-')", i, r)
		}
	}
	
	// No puede terminar con punto
	if strings.HasSuffix(id, ".") {
		return fmt.Errorf("ID de sensor no puede terminar con punto")
	}
	
	// No puede tener puntos consecutivos
	if strings.Contains(id, "..") {
		return fmt.Errorf("ID de sensor no puede tener puntos consecutivos")
	}
	
	return nil
}

// ValidarPatron valida que un patrón de búsqueda sea válido
func ValidarPatron(patron string) error {
	if len(patron) == 0 {
		return fmt.Errorf("patrón vacío")
	}
	
	if len(patron) > 255 {
		return fmt.Errorf("patrón demasiado largo: %d caracteres", len(patron))
	}
	
	// Validar caracteres permitidos en patrón (incluye wildcard *)
	for i, r := range patron {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || 
			 (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' || r == '*') {
			return fmt.Errorf("carácter inválido en patrón en posición %d: '%c'", i, r)
		}
	}
	
	return nil
}

// ValidarRangoTemporal valida que un rango temporal sea lógico
func ValidarRangoTemporal(inicio, fin time.Time) error {
	if inicio.IsZero() && fin.IsZero() {
		return fmt.Errorf("ambos timestamps no pueden ser cero")
	}
	
	if !inicio.IsZero() && !fin.IsZero() && inicio.After(fin) {
		return fmt.Errorf("timestamp de inicio (%v) es posterior al de fin (%v)", inicio, fin)
	}
	
	// Validar que no sea un rango demasiado amplio (más de 10 años)
	if !inicio.IsZero() && !fin.IsZero() {
		duracion := fin.Sub(inicio)
		if duracion > 10*365*24*time.Hour {
			return fmt.Errorf("rango temporal demasiado amplio: %v", duracion)
		}
	}
	
	return nil
}

// ValidarValor valida que un valor numérico sea válido
func ValidarValor(valor float64) error {
	// Verificar NaN
	if valor != valor { // NaN check
		return fmt.Errorf("valor no puede ser NaN")
	}
	
	// Verificar infinito
	if math.IsInf(valor, 0) {
		return fmt.Errorf("valor no puede ser infinito")
	}
	
	return nil
}

// NormalizarIDSensor normaliza un ID de sensor según las mejores prácticas
func NormalizarIDSensor(id string) string {
	// Convertir a minúsculas
	normalized := strings.ToLower(id)
	
	// Reemplazar espacios con guiones bajos
	normalized = strings.ReplaceAll(normalized, " ", "_")
	
	// Reemplazar múltiples guiones bajos consecutivos con uno solo
	re := regexp.MustCompile("_+")
	normalized = re.ReplaceAllString(normalized, "_")
	
	// Remover guiones bajos al inicio y final
	normalized = strings.Trim(normalized, "_")
	
	return normalized
}

// FormatearDuracion formatea una duración de manera legible
func FormatearDuracion(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1e6)
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	} else {
		days := int(d.Hours() / 24)
		hours := d.Hours() - float64(days*24)
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%.1fh", days, hours)
	}
}

// FormatearBytes formatea un número de bytes de manera legible
func FormatearBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// FormatearVelocidad formatea una velocidad (registros por segundo)
func FormatearVelocidad(rps float64) string {
	if rps < 1000 {
		return fmt.Sprintf("%.1f rps", rps)
	} else if rps < 1000000 {
		return fmt.Sprintf("%.1fK rps", rps/1000)
	} else {
		return fmt.Sprintf("%.1fM rps", rps/1000000)
	}
}

// GenerarIDUnico genera un ID único basado en timestamp y un prefijo
func GenerarIDUnico(prefijo string) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s_%d", prefijo, timestamp)
}

// ExtraerPrefijoDePatron extrae la parte fija de un patrón con wildcard
func ExtraerPrefijoDePatron(patron string) string {
	if idx := strings.Index(patron, "*"); idx != -1 {
		return patron[:idx]
	}
	return patron
}

// CrearRangosIntervalo divide un rango temporal en intervalos más pequeños
func CrearRangosIntervalo(inicio, fin time.Time, intervalo time.Duration) [][2]time.Time {
	var rangos [][2]time.Time
	
	actual := inicio
	for actual.Before(fin) {
		siguiente := actual.Add(intervalo)
		if siguiente.After(fin) {
			siguiente = fin
		}
		
		rangos = append(rangos, [2]time.Time{actual, siguiente})
		actual = siguiente
	}
	
	return rangos
}

// CalcularIntervaloPorDefecto calcula un intervalo apropiado basado en la duración total
func CalcularIntervaloPorDefecto(duracionTotal time.Duration) time.Duration {
	if duracionTotal <= time.Hour {
		return time.Minute
	} else if duracionTotal <= 24*time.Hour {
		return time.Hour
	} else if duracionTotal <= 7*24*time.Hour {
		return 6 * time.Hour
	} else if duracionTotal <= 30*24*time.Hour {
		return 24 * time.Hour
	} else {
		return 7 * 24 * time.Hour
	}
}

// TruncarTimestamp trunca un timestamp al intervalo especificado
func TruncarTimestamp(t time.Time, intervalo time.Duration) time.Time {
	switch {
	case intervalo <= time.Second:
		return t.Truncate(time.Second)
	case intervalo <= time.Minute:
		return t.Truncate(time.Minute)
	case intervalo <= time.Hour:
		return t.Truncate(time.Hour)
	case intervalo <= 24*time.Hour:
		// Truncar al día
		year, month, day := t.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	default:
		// Truncar a la semana (lunes)
		year, month, day := t.Date()
		date := time.Date(year, month, day, 0, 0, 0, 0, t.Location())
		// Encontrar el lunes anterior
		weekday := int(date.Weekday())
		if weekday == 0 {
			weekday = 7 // Domingo = 7
		}
		return date.AddDate(0, 0, 1-weekday)
	}
}

// EsPatronValido verifica si un patrón de búsqueda es válido usando regex
func EsPatronValido(patron string) bool {
	// Convertir patrón con wildcards a regex
	regexPattern := strings.ReplaceAll(patron, "*", ".*")
	regexPattern = strings.ReplaceAll(regexPattern, "?", ".")
	
	_, err := regexp.Compile("^" + regexPattern + "$")
	return err == nil
}

// ContarWildcards cuenta el número de wildcards en un patrón
func ContarWildcards(patron string) int {
	return strings.Count(patron, "*") + strings.Count(patron, "?")
}

// EsSensorSimple verifica si un ID es de un sensor simple (sin jerarquía)
func EsSensorSimple(id string) bool {
	return !strings.Contains(id, ".")
}

// ExtraerNivelJerarquia extrae el nivel de jerarquía de un ID de sensor
// Ejemplo: "edificio.planta1.salon.temperatura" -> 4 niveles
func ExtraerNivelJerarquia(id string) int {
	if id == "" {
		return 0
	}
	return strings.Count(id, ".") + 1
}

// ExtraerPrefijos extrae todos los prefijos posibles de un ID jerárquico
// Ejemplo: "a.b.c.d" -> ["a", "a.b", "a.b.c", "a.b.c.d"]
func ExtraerPrefijos(id string) []string {
	if id == "" {
		return []string{}
	}
	
	partes := strings.Split(id, ".")
	prefijos := make([]string, len(partes))
	
	for i := range partes {
		prefijos[i] = strings.Join(partes[:i+1], ".")
	}
	
	return prefijos
}

// EsRangoValido verifica si un rango temporal tiene sentido para consultas
func EsRangoValido(inicio, fin time.Time, limiteDuracion time.Duration) error {
	if err := ValidarRangoTemporal(inicio, fin); err != nil {
		return err
	}
	
	if limiteDuracion > 0 && fin.Sub(inicio) > limiteDuracion {
		return fmt.Errorf("rango temporal %v excede el límite permitido %v", 
			fin.Sub(inicio), limiteDuracion)
	}
	
	return nil
}

// RedondearTimestamp redondea un timestamp al intervalo más cercano
func RedondearTimestamp(t time.Time, intervalo time.Duration) time.Time {
	truncated := TruncarTimestamp(t, intervalo)
	if t.Sub(truncated) >= intervalo/2 {
		return truncated.Add(intervalo)
	}
	return truncated
}