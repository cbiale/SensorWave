package pebble_backend

import (
	"fmt"
	"math"
	"sort"
	"time"

	"edgesensorwave/pkg/motor"
)

// ConsultasAvanzadas proporciona funciones de consulta especializadas
type ConsultasAvanzadas struct {
	db *DB
}

// NuevasConsultasAvanzadas crea una nueva instancia de consultas avanzadas
func (db *DB) NuevasConsultasAvanzadas() *ConsultasAvanzadas {
	return &ConsultasAvanzadas{db: db}
}

// BuscarUltimo busca el último valor de un sensor
func (ca *ConsultasAvanzadas) BuscarUltimo(idSensor string) (*motor.ClaveSensor, *motor.ValorSensor, error) {
	// Crear rango temporal muy amplio hacia atrás desde ahora
	fin := time.Now()
	inicio := time.Time{} // Tiempo mínimo
	
	// Usar iterador reverso para obtener el más reciente
	iter, err := ca.db.ConsultarRangoReverso(idSensor, inicio, fin)
	if err != nil {
		return nil, nil, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	if iter.Siguiente() {
		return iter.Clave(), iter.Valor(), nil
	}
	
	return nil, nil, nil // No encontrado
}

// BuscarPrimero busca el primer valor de un sensor
func (ca *ConsultasAvanzadas) BuscarPrimero(idSensor string) (*motor.ClaveSensor, *motor.ValorSensor, error) {
	// Crear rango temporal muy amplio hacia adelante desde el inicio
	inicio := time.Time{} // Tiempo mínimo
	fin := time.Now().Add(24 * time.Hour) // Un poco en el futuro
	
	iter, err := ca.db.ConsultarRangoConLimite(idSensor, inicio, fin, 1)
	if err != nil {
		return nil, nil, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	if iter.Siguiente() {
		return iter.Clave(), iter.Valor(), nil
	}
	
	return nil, nil, nil // No encontrado
}

// EstadisticasSensor contiene estadísticas de un sensor
type EstadisticasSensor struct {
	ID               string
	NumRegistros     int64
	ValorMinimo      float64
	ValorMaximo      float64
	ValorPromedio    float64
	PrimerTimestamp  time.Time
	UltimoTimestamp  time.Time
	CalidadDistrib   map[motor.CalidadDato]int64
}

// CalcularEstadisticas calcula estadísticas para un sensor en un rango temporal
func (ca *ConsultasAvanzadas) CalcularEstadisticas(idSensor string, inicio, fin time.Time) (*EstadisticasSensor, error) {
	iter, err := ca.db.ConsultarRango(idSensor, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	stats := &EstadisticasSensor{
		ID:             idSensor,
		ValorMinimo:    math.Inf(1),  // +Inf
		ValorMaximo:    math.Inf(-1), // -Inf
		CalidadDistrib: make(map[motor.CalidadDato]int64),
	}
	
	var suma float64
	var primero = true
	
	for iter.Siguiente() {
		clave := iter.Clave()
		valor := iter.Valor()
		
		// Actualizar contadores
		stats.NumRegistros++
		suma += valor.Valor
		
		// Actualizar min/max
		if valor.Valor < stats.ValorMinimo {
			stats.ValorMinimo = valor.Valor
		}
		if valor.Valor > stats.ValorMaximo {
			stats.ValorMaximo = valor.Valor
		}
		
		// Actualizar timestamps
		if primero {
			stats.PrimerTimestamp = clave.Timestamp
			primero = false
		}
		stats.UltimoTimestamp = clave.Timestamp
		
		// Distribución de calidad
		stats.CalidadDistrib[valor.Calidad]++
	}
	
	// Calcular promedio
	if stats.NumRegistros > 0 {
		stats.ValorPromedio = suma / float64(stats.NumRegistros)
	}
	
	// Si no hay registros, ajustar min/max
	if stats.NumRegistros == 0 {
		stats.ValorMinimo = 0
		stats.ValorMaximo = 0
	}
	
	return stats, nil
}

// ListarSensores retorna una lista de todos los sensores únicos que coincidan con un patrón
func (ca *ConsultasAvanzadas) ListarSensores(patron string) ([]string, error) {
	// Usar un rango temporal muy amplio
	inicio := time.Time{}
	fin := time.Now().Add(24 * time.Hour)
	
	iter, err := ca.db.ConsultarRango(patron, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	sensoresSet := make(map[string]bool)
	
	for iter.Siguiente() {
		clave := iter.Clave()
		sensoresSet[clave.IDSensor] = true
	}
	
	// Convertir a slice y ordenar
	sensores := make([]string, 0, len(sensoresSet))
	for sensor := range sensoresSet {
		sensores = append(sensores, sensor)
	}
	
	sort.Strings(sensores)
	return sensores, nil
}

// AgregarDatos representa datos agregados (promedio, suma, etc.)
type DatosAgregados struct {
	Timestamp    time.Time
	Promedio     float64
	Minimo       float64
	Maximo       float64
	Suma         float64
	Conteo       int64
	Desviacion   float64
}

// AgregarPorIntervalo agrupa datos por intervalos de tiempo y calcula estadísticas
func (ca *ConsultasAvanzadas) AgregarPorIntervalo(idSensor string, inicio, fin time.Time, intervalo time.Duration) ([]DatosAgregados, error) {
	iter, err := ca.db.ConsultarRango(idSensor, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	// Mapa para agrupar por intervalos
	intervalos := make(map[int64][]float64)
	
	for iter.Siguiente() {
		clave := iter.Clave()
		valor := iter.Valor()
		
		// Calcular bucket de intervalo
		bucket := clave.Timestamp.UnixNano() / int64(intervalo)
		intervalos[bucket] = append(intervalos[bucket], valor.Valor)
	}
	
	// Convertir a resultado ordenado
	var buckets []int64
	for bucket := range intervalos {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })
	
	resultado := make([]DatosAgregados, 0, len(buckets))
	
	for _, bucket := range buckets {
		valores := intervalos[bucket]
		if len(valores) == 0 {
			continue
		}
		
		// Calcular estadísticas
		agregado := DatosAgregados{
			Timestamp: time.Unix(0, bucket*int64(intervalo)),
			Conteo:    int64(len(valores)),
			Minimo:    math.Inf(1),
			Maximo:    math.Inf(-1),
		}
		
		// Calcular suma, min, max
		for _, v := range valores {
			agregado.Suma += v
			if v < agregado.Minimo {
				agregado.Minimo = v
			}
			if v > agregado.Maximo {
				agregado.Maximo = v
			}
		}
		
		// Calcular promedio
		agregado.Promedio = agregado.Suma / float64(agregado.Conteo)
		
		// Calcular desviación estándar
		var sumaCuadrados float64
		for _, v := range valores {
			diff := v - agregado.Promedio
			sumaCuadrados += diff * diff
		}
		agregado.Desviacion = math.Sqrt(sumaCuadrados / float64(agregado.Conteo))
		
		resultado = append(resultado, agregado)
	}
	
	return resultado, nil
}

// BuscarAnomalias busca valores que se desvían significativamente del promedio
func (ca *ConsultasAvanzadas) BuscarAnomalias(idSensor string, inicio, fin time.Time, umbralDesviaciones float64) ([]motor.ClaveSensor, error) {
	// Primero calcular estadísticas del período
	stats, err := ca.CalcularEstadisticas(idSensor, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error calculando estadísticas: %w", err)
	}
	
	if stats.NumRegistros == 0 {
		return []motor.ClaveSensor{}, nil
	}
	
	// Calcular desviación estándar
	iter, err := ca.db.ConsultarRango(idSensor, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador para desviación: %w", err)
	}
	defer iter.Cerrar()
	
	var sumaCuadrados float64
	for iter.Siguiente() {
		valor := iter.Valor()
		diff := valor.Valor - stats.ValorPromedio
		sumaCuadrados += diff * diff
	}
	
	desviacion := math.Sqrt(sumaCuadrados / float64(stats.NumRegistros))
	umbral := desviacion * umbralDesviaciones
	
	// Segunda pasada para encontrar anomalías
	iter2, err := ca.db.ConsultarRango(idSensor, inicio, fin)
	if err != nil {
		return nil, fmt.Errorf("error creando iterador para anomalías: %w", err)
	}
	defer iter2.Cerrar()
	
	var anomalias []motor.ClaveSensor
	
	for iter2.Siguiente() {
		clave := iter2.Clave()
		valor := iter2.Valor()
		
		diff := math.Abs(valor.Valor - stats.ValorPromedio)
		if diff > umbral {
			anomalias = append(anomalias, *clave)
		}
	}
	
	return anomalias, nil
}

// ResultadoBusqueda representa un resultado de búsqueda múltiple
type ResultadoBusqueda struct {
	IDSensor   string
	Clave      *motor.ClaveSensor
	Valor      *motor.ValorSensor
	Relevancia float64 // Score de relevancia (0-1)
}

// BusquedaMultiple busca en múltiples sensores con scoring de relevancia
func (ca *ConsultasAvanzadas) BusquedaMultiple(patrones []string, inicio, fin time.Time, limite int) ([]ResultadoBusqueda, error) {
	var resultados []ResultadoBusqueda
	
	for _, patron := range patrones {
		iter, err := ca.db.ConsultarRango(patron, inicio, fin)
		if err != nil {
			continue // Continuar con otros patrones en caso de error
		}
		
		for iter.Siguiente() {
			clave := iter.Clave()
			valor := iter.Valor()
			
			// Calcular relevancia basada en calidad y recencia
			relevancia := calcularRelevancia(valor, clave.Timestamp, fin)
			
			resultados = append(resultados, ResultadoBusqueda{
				IDSensor:   clave.IDSensor,
				Clave:      clave,
				Valor:      valor,
				Relevancia: relevancia,
			})
		}
		
		iter.Cerrar()
	}
	
	// Ordenar por relevancia descendente
	sort.Slice(resultados, func(i, j int) bool {
		return resultados[i].Relevancia > resultados[j].Relevancia
	})
	
	// Aplicar límite
	if limite > 0 && len(resultados) > limite {
		resultados = resultados[:limite]
	}
	
	return resultados, nil
}

// calcularRelevancia calcula un score de relevancia para un registro
func calcularRelevancia(valor *motor.ValorSensor, timestamp, referencia time.Time) float64 {
	// Score base por calidad
	var scoreCalidad float64
	switch valor.Calidad {
	case motor.CalidadBuena:
		scoreCalidad = 1.0
	case motor.CalidadSospechosa:
		scoreCalidad = 0.7
	case motor.CalidadMala:
		scoreCalidad = 0.3
	case motor.CalidadDesconocida:
		scoreCalidad = 0.1
	}
	
	// Score por recencia (más reciente = mayor score)
	duracion := referencia.Sub(timestamp)
	scoreRecencia := math.Exp(-duracion.Hours() / 24.0) // Decae exponencialmente por día
	
	// Score combinado
	return (scoreCalidad * 0.7) + (scoreRecencia * 0.3)
}

// ConsultarEnVentana consulta datos en una ventana deslizante
func (ca *ConsultasAvanzadas) ConsultarEnVentana(idSensor string, centro time.Time, ventana time.Duration) (motor.Iterador, error) {
	mitadVentana := ventana / 2
	inicio := centro.Add(-mitadVentana)
	fin := centro.Add(mitadVentana)
	
	return ca.db.ConsultarRango(idSensor, inicio, fin)
}

// ContarRegistros cuenta el número de registros que coinciden con el patrón en el rango
func (ca *ConsultasAvanzadas) ContarRegistros(patron string, inicio, fin time.Time) (int64, error) {
	iter, err := ca.db.ConsultarRango(patron, inicio, fin)
	if err != nil {
		return 0, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	var contador int64
	for iter.Siguiente() {
		contador++
	}
	
	return contador, nil
}

// Verificar si un sensor tiene datos en un rango
func (ca *ConsultasAvanzadas) TieneDatos(idSensor string, inicio, fin time.Time) (bool, error) {
	iter, err := ca.db.ConsultarRangoConLimite(idSensor, inicio, fin, 1)
	if err != nil {
		return false, fmt.Errorf("error creando iterador: %w", err)
	}
	defer iter.Cerrar()
	
	return iter.Siguiente(), nil
}