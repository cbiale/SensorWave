package edge

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/cbiale/sensorwave/tipos"
)

// validarPuertoHTTP valida que el puerto HTTP sea correcto
func validarPuertoHTTP(puerto string) (string, error) {
	if puerto == "" {
		return "8080", nil // Puerto por defecto
	}

	// Validar que sea un número válido
	puertoNum, err := strconv.Atoi(puerto)
	if err != nil {
		return "", fmt.Errorf("puerto HTTP inválido: debe ser un número")
	}

	// Validar rango de puerto
	if puertoNum < 1 || puertoNum > 65535 {
		return "", fmt.Errorf("puerto HTTP fuera de rango: debe estar entre 1 y 65535")
	}

	// Advertir sobre puertos reservados
	if puertoNum < 1024 {
		log.Printf("Advertencia: el puerto %d es un puerto reservado", puertoNum)
	}

	return puerto, nil
}

// obtenerIPPrincipal obtiene la dirección IP principal del nodo
func obtenerIPPrincipal() (string, error) {
	// Crear una conexión UDP (no se envía realmente)
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("error al determinar IP: %v", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// generarNodoID genera un ID único para el nodo edge
func generarNodoID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	UUID := uuid.New().String()
	return fmt.Sprintf("edge-%s-%s", hostname, UUID)
}

// generarClaveDatos genera una clave PebbleDB incluyendo el tipo de datos
func generarClaveDatos(serieId int, tiempoInicio, tiempoFin int64) []byte {
	key := fmt.Sprintf("data/%010d/%020d_%020d", serieId, tiempoInicio, tiempoFin)
	return []byte(key)
}

// esPathValido valida que un path de serie tenga el formato correcto
func esPathValido(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return false
	}

	// Verificar que no tenga componentes vacíos
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" {
			return false
		}
		// Verificar caracteres válidos (solo alfanuméricos y _)
		if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(part) {
			return false
		}
	}
	return true
}

// inferirTipo determina el tipo de datos basado en el valor proporcionado
func inferirTipo(valor interface{}) tipos.TipoDatos {
	switch valor.(type) {
	case bool:
		return tipos.Boolean
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return tipos.Integer
	case float32, float64:
		return tipos.Real
	case string:
		return tipos.Text
	default:
		return tipos.Desconocido
	}
}

// esCompatibleConTipo verifica si un valor es compatible con el tipo de serie
func esCompatibleConTipo(valor interface{}, tipoDatos tipos.TipoDatos) bool {
	tipoValor := inferirTipo(valor)

	// Si el tipo inferido es desconocido, no es compatible
	if tipoValor == tipos.Desconocido {
		return false
	}

	// Verificar compatibilidad exacta
	return tipoValor == tipoDatos
}

// matchTags verifica si una serie tiene todos los tags especificados
func matchTags(serieTags, filterTags map[string]string) bool {
	if len(filterTags) == 0 {
		return true
	}

	for key, value := range filterTags {
		if serieValue, existe := serieTags[key]; !existe || serieValue != value {
			return false
		}
	}

	return true
}
