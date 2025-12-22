package edge

// Package edge - funciones de consulta para series temporales.
// Este archivo contiene las operaciones de lectura y consulta sobre datos almacenados.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cbiale/sensorwave/tipos"
	"github.com/cockroachdb/pebble"
)

// ConsultarRango consulta mediciones de una o más series dentro de un rango de tiempo
// y descomprime los datos antes de retornarlos.
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura" o "*/temperatura"
func (me *ManagerEdge) ConsultarRango(path string, tiempoInicio, tiempoFin time.Time) ([]tipos.Medicion, error) {
	// Resolver series (path exacto o patrón wildcard)
	series, err := me.resolverSeries(path)
	if err != nil {
		return nil, err
	}

	// Recolectar y combinar resultados de todas las series
	var todasMediciones []tipos.Medicion
	for _, serie := range series {
		mediciones, err := me.consultarRangoSerie(serie, tiempoInicio, tiempoFin)
		if err != nil {
			continue // Ignorar series sin datos
		}
		todasMediciones = append(todasMediciones, mediciones...)
	}

	return todasMediciones, nil
}

// consultarRangoSerie consulta mediciones de una serie específica dentro de un rango de tiempo
func (me *ManagerEdge) consultarRangoSerie(serie tipos.Serie, tiempoInicio, tiempoFin time.Time) ([]tipos.Medicion, error) {
	// Convertir los tiempos a Unix timestamp (en nanosegundos)
	tiempoInicioUnix := tiempoInicio.UnixNano()
	tiempoFinUnix := tiempoFin.UnixNano()

	var resultados []tipos.Medicion

	// Crear rangos de búsqueda para iterar sobre los datos de la serie
	keyPrefix := fmt.Sprintf("data/%010d/", serie.SerieId)
	lowerBound := []byte(keyPrefix)
	upperBound := []byte(keyPrefix + "~") // '~' es mayor que todos los números

	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Iterar sobre todos los bloques de la serie
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())

		// Extraer timestamps del rango del bloque desde la clave para skip temprano
		// Formato: data/XXXXXXXXXX/TTTTTTTTTTTTTTTTTTTT_TTTTTTTTTTTTTTTTTTTT
		if skipBloque := me.deberiaSkipearBloque(key, tiempoInicioUnix, tiempoFinUnix); skipBloque {
			continue // Skip este bloque sin descomprimirlo
		}

		datosComprimidos := make([]byte, len(iter.Value()))
		copy(datosComprimidos, iter.Value())

		// Descomprimir el bloque
		mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
		if err != nil {
			fmt.Printf("Error al descomprimir bloque: %v\n", err)
			continue
		}

		// Filtrar mediciones que están dentro del rango solicitado
		for _, medicion := range mediciones {
			if medicion.Tiempo >= tiempoInicioUnix && medicion.Tiempo <= tiempoFinUnix {
				resultados = append(resultados, medicion)
			}
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("error al iterar sobre datos: %v", err)
	}

	// Obtener datos del buffer en memoria si existe
	if bufferInterface, ok := me.buffers.Load(serie.Path); ok {
		buffer := bufferInterface.(*SerieBuffer)
		buffer.mu.Lock()

		// Revisar datos del buffer que están dentro del rango
		for i := 0; i < buffer.indice; i++ {
			medicion := buffer.datos[i]
			if medicion.Tiempo >= tiempoInicioUnix && medicion.Tiempo <= tiempoFinUnix {
				resultados = append(resultados, medicion)
			}
		}

		buffer.mu.Unlock()
	}

	return resultados, nil
}

// ConsultarUltimoPunto obtiene la última medición registrada para una o más series.
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura" o "*/temperatura"
//
// Si se usa wildcard, retorna la medición más reciente entre todas las series que coincidan.
func (me *ManagerEdge) ConsultarUltimoPunto(path string) (tipos.Medicion, error) {
	// Resolver series (path exacto o patrón wildcard)
	series, err := me.resolverSeries(path)
	if err != nil {
		return tipos.Medicion{}, err
	}

	// Encontrar la medición más reciente entre todas las series
	var ultimaMedicion tipos.Medicion
	encontrado := false

	for _, serie := range series {
		medicion, err := me.consultarUltimoPuntoSerie(serie)
		if err != nil {
			continue // Ignorar series sin datos
		}
		if !encontrado || medicion.Tiempo > ultimaMedicion.Tiempo {
			ultimaMedicion = medicion
			encontrado = true
		}
	}

	if !encontrado {
		return tipos.Medicion{}, fmt.Errorf("no hay mediciones para el patrón: %s", path)
	}

	return ultimaMedicion, nil
}

// consultarUltimoPuntoSerie obtiene la última medición de una serie específica
func (me *ManagerEdge) consultarUltimoPuntoSerie(serie tipos.Serie) (tipos.Medicion, error) {
	// Primero revisar el buffer en memoria
	if bufferInterface, ok := me.buffers.Load(serie.Path); ok {
		buffer := bufferInterface.(*SerieBuffer)
		buffer.mu.Lock()
		defer buffer.mu.Unlock()

		if buffer.indice > 0 {
			// Encontrar la medición más reciente en el buffer
			ultimaMedicion := buffer.datos[0]
			for i := 1; i < buffer.indice; i++ {
				if buffer.datos[i].Tiempo > ultimaMedicion.Tiempo {
					ultimaMedicion = buffer.datos[i]
				}
			}
			return ultimaMedicion, nil
		}
	}

	// Buscar el último bloque para esta serie
	keyPrefix := fmt.Sprintf("data/%010d/", serie.SerieId)
	lowerBound := []byte(keyPrefix)
	upperBound := []byte(keyPrefix + "~")

	iter, err := me.db.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("error al crear iterador: %v", err)
	}
	defer iter.Close()

	// Ir al último elemento
	if !iter.Last() {
		return tipos.Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", serie.Path)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("error al descomprimir último bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return tipos.Medicion{}, fmt.Errorf("bloque vacío para serie: %s", serie.Path)
	}

	// Encontrar la medición más reciente en el bloque
	ultimaMedicion := mediciones[0]
	for _, m := range mediciones[1:] {
		if m.Tiempo > ultimaMedicion.Tiempo {
			ultimaMedicion = m
		}
	}

	return ultimaMedicion, nil
}

// deberiaSkipearBloque determina si un bloque debe ser omitido basado en su rango temporal
func (me *ManagerEdge) deberiaSkipearBloque(clave string, tiempoInicio, tiempoFin int64) bool {
	partes := strings.Split(clave, "/")

	// Formato: data/XXXXXXXXXX/TTTTTTTTTTTTTTTTTTTT_TTTTTTTTTTTTTTTTTTTT
	if len(partes) != 3 {
		return false // Formato desconocido, no skip
	}

	tiempoRango := partes[2] // Índice correcto: 0=data, 1=serieID, 2=rango temporal
	tiempoPartes := strings.Split(tiempoRango, "_")
	if len(tiempoPartes) != 2 {
		return false // Formato sin rango, no skip
	}

	bloqueInicio, err1 := strconv.ParseInt(tiempoPartes[0], 10, 64)
	bloqueFin, err2 := strconv.ParseInt(tiempoPartes[1], 10, 64)

	if err1 != nil || err2 != nil {
		return false // Error parsing, no skip por seguridad
	}

	// Skip si no hay superposición temporal:
	// El bloque termina antes que inicie nuestro rango, O
	// El bloque inicia después que termine nuestro rango
	if bloqueFin < tiempoInicio || bloqueInicio > tiempoFin {
		return true
	}

	return false
}

// resolverSeries resuelve un path (exacto o con patrón wildcard) a una lista de series.
// Si el path contiene "*", busca por patrón usando ListarSeriesPorPath.
// Si no, busca la serie exacta usando ObtenerSeries.
func (me *ManagerEdge) resolverSeries(path string) ([]tipos.Serie, error) {
	if strings.Contains(path, "*") {
		series, err := me.ListarSeriesPorPath(path)
		if err != nil {
			return nil, err
		}
		if len(series) == 0 {
			return nil, fmt.Errorf("no se encontraron series para el patrón: %s", path)
		}
		return series, nil
	}

	// Path exacto
	serie, err := me.ObtenerSeries(path)
	if err != nil {
		return nil, fmt.Errorf("serie no encontrada: %s", path)
	}
	return []tipos.Serie{serie}, nil
}

// ConsultarAgregacion calcula una agregación sobre una o más series.
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura" o "*/temperatura"
//
// Retorna el valor agregado de todas las mediciones en el rango temporal.
func (me *ManagerEdge) ConsultarAgregacion(
	path string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
) (float64, error) {
	// Resolver series (path exacto o patrón)
	series, err := me.resolverSeries(path)
	if err != nil {
		return 0, err
	}

	// Recolectar todos los valores de todas las series
	var todosLosValores []float64

	for _, serie := range series {
		mediciones, err := me.ConsultarRango(serie.Path, tiempoInicio, tiempoFin)
		if err != nil {
			continue // Ignorar series sin datos
		}

		for _, medicion := range mediciones {
			valorFloat, err := convertirAFloat64(medicion.Valor)
			if err != nil {
				// Para COUNT, contar cualquier valor como 1
				if agregacion == tipos.AgregacionCount {
					valorFloat = 1.0
				} else {
					continue // Saltar valores no numéricos para otras agregaciones
				}
			}
			todosLosValores = append(todosLosValores, valorFloat)
		}
	}

	if len(todosLosValores) == 0 {
		return 0, fmt.Errorf("no hay datos en el rango especificado para: %s", path)
	}

	return CalcularAgregacionSimple(todosLosValores, agregacion)
}

// ConsultarAgregacionTemporal calcula agregaciones agrupadas por intervalos de tiempo (downsampling).
// Retorna un slice con un valor agregado por cada bucket temporal.
//
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura"
//
// Ejemplo: ConsultarAgregacionTemporal("sensor_01/temp", inicio, fin, AgregacionPromedio, time.Hour)
// retorna el promedio por cada hora en el rango.
func (me *ManagerEdge) ConsultarAgregacionTemporal(
	path string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
	intervalo time.Duration,
) ([]tipos.ResultadoAgregacionTemporal, error) {
	if intervalo <= 0 {
		return nil, fmt.Errorf("el intervalo debe ser mayor a cero")
	}

	// Resolver series (path exacto o patrón)
	series, err := me.resolverSeries(path)
	if err != nil {
		return nil, err
	}

	// Recolectar todas las mediciones de todas las series
	var todasLasMediciones []tipos.Medicion

	for _, serie := range series {
		mediciones, err := me.ConsultarRango(serie.Path, tiempoInicio, tiempoFin)
		if err != nil {
			continue
		}
		todasLasMediciones = append(todasLasMediciones, mediciones...)
	}

	if len(todasLasMediciones) == 0 {
		return nil, fmt.Errorf("no hay datos en el rango especificado para: %s", path)
	}

	// Crear buckets temporales
	var resultados []tipos.ResultadoAgregacionTemporal
	bucketInicio := tiempoInicio

	for bucketInicio.Before(tiempoFin) {
		bucketFin := bucketInicio.Add(intervalo)
		if bucketFin.After(tiempoFin) {
			bucketFin = tiempoFin
		}

		// Filtrar mediciones dentro de este bucket
		bucketInicioNano := bucketInicio.UnixNano()
		bucketFinNano := bucketFin.UnixNano()

		var valoresBucket []float64
		for _, medicion := range todasLasMediciones {
			if medicion.Tiempo >= bucketInicioNano && medicion.Tiempo < bucketFinNano {
				valorFloat, err := convertirAFloat64(medicion.Valor)
				if err != nil {
					if agregacion == tipos.AgregacionCount {
						valorFloat = 1.0
					} else {
						continue
					}
				}
				valoresBucket = append(valoresBucket, valorFloat)
			}
		}

		// Calcular agregación para este bucket si tiene datos
		if len(valoresBucket) > 0 {
			valor, err := CalcularAgregacionSimple(valoresBucket, agregacion)
			if err == nil {
				resultados = append(resultados, tipos.ResultadoAgregacionTemporal{
					Tiempo: bucketInicio,
					Valor:  valor,
				})
			}
		}

		bucketInicio = bucketFin
	}

	return resultados, nil
}
