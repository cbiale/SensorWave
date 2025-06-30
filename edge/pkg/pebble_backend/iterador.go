package pebble_backend

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"edgesensorwave/pkg/motor"
)

// IteradorPebble implementa motor.Iterador usando pebble.Iterator
type IteradorPebble struct {
	iter         *pebble.Iterator
	patron       string
	claveActual  *motor.ClaveSensor
	valorActual  *motor.ValorSensor
	hayError     error
	cerrado      bool
}

// ConsultarRango retorna un iterador para consultar datos en un rango temporal
func (db *DB) ConsultarRango(patron string, inicio, fin time.Time) (motor.Iterador, error) {
	return db.ConsultarRangoConLimite(patron, inicio, fin, -1)
}

// ConsultarRangoConLimite retorna un iterador con límite de resultados
func (db *DB) ConsultarRangoConLimite(patron string, inicio, fin time.Time, limite int) (motor.Iterador, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	if db.cerrada {
		return nil, fmt.Errorf("base de datos cerrada")
	}
	
	// Determinar bounds para la búsqueda
	var lowerBound, upperBound []byte
	
	if strings.Contains(patron, "*") {
		// Patrón con wildcard - usar prefijo
		prefijo := strings.TrimSuffix(patron, "*")
		lowerBound = crearBoundConPrefijo(prefijo, inicio)
		upperBound = crearBoundConPrefijo(prefijo, fin)
	} else {
		// Sensor específico
		lowerBound = CrearClaveRango(patron, inicio)
		upperBound = CrearClaveRango(patron, fin)
	}
	
	// Crear iterador con bounds
	iter, err := db.pb.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %w", err)
	}
	
	iterador := &IteradorPebble{
		iter:   iter,
		patron: patron,
	}
	
	// Posicionar al inicio
	if !iter.First() {
		iter.Close()
		return &IteradorVacio{}, nil
	}
	
	// Aplicar límite si se especifica
	if limite > 0 {
		return &IteradorConLimite{
			iterador: iterador,
			limite:   limite,
			contador: 0,
		}, nil
	}
	
	return iterador, nil
}

// Siguiente avanza al siguiente elemento que coincida con el patrón
func (i *IteradorPebble) Siguiente() bool {
	if i.cerrado || i.hayError != nil {
		return false
	}
	
	for i.iter.Valid() {
		// Deserializar clave
		clave, err := DeserializarClave(i.iter.Key())
		if err != nil {
			i.hayError = fmt.Errorf("error deserializando clave: %w", err)
			return false
		}
		
		// Verificar si la clave coincide con el patrón
		if !coincidePatron(clave.IDSensor, i.patron) {
			if !i.iter.Next() {
				return false
			}
			continue
		}
		
		// Deserializar valor
		valor, err := DeserializarValor(i.iter.Value())
		if err != nil {
			i.hayError = fmt.Errorf("error deserializando valor: %w", err)
			return false
		}
		
		// Almacenar resultado actual
		i.claveActual = clave
		i.valorActual = valor
		
		// Avanzar para la próxima llamada
		i.iter.Next()
		
		return true
	}
	
	// Verificar errores del iterador
	if err := i.iter.Error(); err != nil {
		i.hayError = err
		return false
	}
	
	return false
}

// Clave retorna la clave actual del iterador
func (i *IteradorPebble) Clave() *motor.ClaveSensor {
	return i.claveActual
}

// Valor retorna el valor actual del iterador
func (i *IteradorPebble) Valor() *motor.ValorSensor {
	return i.valorActual
}

// Cerrar cierra el iterador y libera recursos
func (i *IteradorPebble) Cerrar() error {
	if !i.cerrado {
		i.cerrado = true
		if i.iter != nil {
			return i.iter.Close()
		}
	}
	return nil
}

// IteradorConLimite implementa un iterador con límite de resultados
type IteradorConLimite struct {
	iterador motor.Iterador
	limite   int
	contador int
}

func (i *IteradorConLimite) Siguiente() bool {
	if i.contador >= i.limite {
		return false
	}
	
	if i.iterador.Siguiente() {
		i.contador++
		return true
	}
	
	return false
}

func (i *IteradorConLimite) Clave() *motor.ClaveSensor {
	return i.iterador.Clave()
}

func (i *IteradorConLimite) Valor() *motor.ValorSensor {
	return i.iterador.Valor()
}

func (i *IteradorConLimite) Cerrar() error {
	return i.iterador.Cerrar()
}

// IteradorVacio implementa un iterador que no retorna resultados
type IteradorVacio struct{}

func (i *IteradorVacio) Siguiente() bool                     { return false }
func (i *IteradorVacio) Clave() *motor.ClaveSensor          { return nil }
func (i *IteradorVacio) Valor() *motor.ValorSensor          { return nil }
func (i *IteradorVacio) Cerrar() error                      { return nil }

// IteradorFiltrado implementa un iterador con filtros adicionales
type IteradorFiltrado struct {
	iterador       motor.Iterador
	filtroCalidad  *motor.CalidadDato
	filtroMetadata map[string]string
}

func (i *IteradorFiltrado) Siguiente() bool {
	for i.iterador.Siguiente() {
		valor := i.iterador.Valor()
		
		// Aplicar filtro de calidad si está definido
		if i.filtroCalidad != nil && valor.Calidad != *i.filtroCalidad {
			continue
		}
		
		// Aplicar filtro de metadatos si está definido
		if i.filtroMetadata != nil && !coincideMetadata(valor.Metadatos, i.filtroMetadata) {
			continue
		}
		
		return true
	}
	
	return false
}

func (i *IteradorFiltrado) Clave() *motor.ClaveSensor {
	return i.iterador.Clave()
}

func (i *IteradorFiltrado) Valor() *motor.ValorSensor {
	return i.iterador.Valor()
}

func (i *IteradorFiltrado) Cerrar() error {
	return i.iterador.Cerrar()
}

// ConsultarConFiltros crea un iterador con filtros de calidad y metadatos
func (db *DB) ConsultarConFiltros(patron string, inicio, fin time.Time, filtroCalidad *motor.CalidadDato, filtroMetadata map[string]string) (motor.Iterador, error) {
	// Crear iterador base
	iterBase, err := db.ConsultarRango(patron, inicio, fin)
	if err != nil {
		return nil, err
	}
	
	// Si no hay filtros, retornar iterador base
	if filtroCalidad == nil && filtroMetadata == nil {
		return iterBase, nil
	}
	
	// Crear iterador filtrado
	return &IteradorFiltrado{
		iterador:       iterBase,
		filtroCalidad:  filtroCalidad,
		filtroMetadata: filtroMetadata,
	}, nil
}

// Funciones auxiliares

// crearBoundConPrefijo crea bounds para búsqueda con prefijo
func crearBoundConPrefijo(prefijo string, timestamp time.Time) []byte {
	clave := &motor.ClaveSensor{
		IDSensor:  prefijo,
		Timestamp: timestamp,
	}
	return SerializarClave(clave)
}

// coincidePatron verifica si un ID de sensor coincide con un patrón
func coincidePatron(idSensor, patron string) bool {
	if patron == "*" {
		return true
	}
	
	if !strings.Contains(patron, "*") {
		return idSensor == patron
	}
	
	// Usar filepath.Match para patrones con wildcards
	matched, err := filepath.Match(patron, idSensor)
	if err != nil {
		// En caso de error en el patrón, hacer coincidencia por prefijo
		prefijo := strings.TrimSuffix(patron, "*")
		return strings.HasPrefix(idSensor, prefijo)
	}
	
	return matched
}

// coincideMetadata verifica si los metadatos coinciden con el filtro
func coincideMetadata(metadatos, filtro map[string]string) bool {
	if metadatos == nil {
		return len(filtro) == 0
	}
	
	for clave, valor := range filtro {
		if metadatos[clave] != valor {
			return false
		}
	}
	
	return true
}

// IteradorReverso implementa iteración en orden reverso
type IteradorReverso struct {
	iter         *pebble.Iterator
	patron       string
	claveActual  *motor.ClaveSensor
	valorActual  *motor.ValorSensor
	hayError     error
	cerrado      bool
}

// ConsultarRangoReverso retorna un iterador que recorre en orden temporal inverso
func (db *DB) ConsultarRangoReverso(patron string, inicio, fin time.Time) (motor.Iterador, error) {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	
	if db.cerrada {
		return nil, fmt.Errorf("base de datos cerrada")
	}
	
	// Crear bounds (invertidos para orden reverso)
	var lowerBound, upperBound []byte
	
	if strings.Contains(patron, "*") {
		prefijo := strings.TrimSuffix(patron, "*")
		lowerBound = crearBoundConPrefijo(prefijo, inicio)
		upperBound = crearBoundConPrefijo(prefijo, fin)
	} else {
		lowerBound = CrearClaveRango(patron, inicio)
		upperBound = CrearClaveRango(patron, fin)
	}
	
	iter, err := db.pb.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, fmt.Errorf("error creando iterador reverso: %w", err)
	}
	
	iterador := &IteradorReverso{
		iter:   iter,
		patron: patron,
	}
	
	// Posicionar al final para orden reverso
	if !iter.Last() {
		iter.Close()
		return &IteradorVacio{}, nil
	}
	
	return iterador, nil
}

func (i *IteradorReverso) Siguiente() bool {
	if i.cerrado || i.hayError != nil {
		return false
	}
	
	for i.iter.Valid() {
		clave, err := DeserializarClave(i.iter.Key())
		if err != nil {
			i.hayError = fmt.Errorf("error deserializando clave: %w", err)
			return false
		}
		
		if !coincidePatron(clave.IDSensor, i.patron) {
			if !i.iter.Prev() {
				return false
			}
			continue
		}
		
		valor, err := DeserializarValor(i.iter.Value())
		if err != nil {
			i.hayError = fmt.Errorf("error deserializando valor: %w", err)
			return false
		}
		
		i.claveActual = clave
		i.valorActual = valor
		
		i.iter.Prev() // Retroceder para orden inverso
		
		return true
	}
	
	if err := i.iter.Error(); err != nil {
		i.hayError = err
		return false
	}
	
	return false
}

func (i *IteradorReverso) Clave() *motor.ClaveSensor {
	return i.claveActual
}

func (i *IteradorReverso) Valor() *motor.ValorSensor {
	return i.valorActual
}

func (i *IteradorReverso) Cerrar() error {
	if !i.cerrado {
		i.cerrado = true
		if i.iter != nil {
			return i.iter.Close()
		}
	}
	return nil
}