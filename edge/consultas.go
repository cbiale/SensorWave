package edge

// Package edge - funciones de consulta para series temporales.
// Este archivo contiene las operaciones de lectura y consulta sobre datos almacenados.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cbiale/sensorwave/tipos"
	"github.com/cockroachdb/pebble"
)

// ConsultarRango consulta mediciones de una o más series dentro de un rango de tiempo
// y retorna los resultados en formato tabular.
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura", "*/temperatura" o "sensor_1/*"
//
// El resultado es una matriz donde:
//   - Cada columna representa una serie (ordenadas alfabéticamente por path)
//   - Cada fila representa un timestamp único (ordenados ascendente)
//   - Los valores faltantes se representan como nil
func (me *ManagerEdge) ConsultarRango(path string, tiempoInicio, tiempoFin time.Time) (tipos.ResultadoConsultaRango, error) {
	// Resolver series (path exacto o patrón wildcard)
	series, err := me.resolverSeries(path)
	if err != nil {
		return tipos.ResultadoConsultaRango{}, err
	}

	// Mapa para almacenar mediciones por serie: path -> timestamp -> valor
	medicionesPorSerie := make(map[string]map[int64]interface{})
	timestampsUnicos := make(map[int64]struct{})

	// Recolectar mediciones de cada serie
	for _, serie := range series {
		mediciones, err := me.consultarRangoSerie(serie, tiempoInicio, tiempoFin)
		if err != nil {
			continue // Ignorar series con error
		}

		// Solo agregar la serie si tiene mediciones
		if len(mediciones) == 0 {
			continue
		}

		// Inicializar mapa para esta serie
		if medicionesPorSerie[serie.Path] == nil {
			medicionesPorSerie[serie.Path] = make(map[int64]interface{})
		}

		// Almacenar cada medición
		for _, m := range mediciones {
			medicionesPorSerie[serie.Path][m.Tiempo] = m.Valor
			timestampsUnicos[m.Tiempo] = struct{}{}
		}
	}

	// Construir resultado tabular
	return construirResultadoTabular(medicionesPorSerie, timestampsUnicos), nil
}

// construirResultadoTabular convierte las mediciones por serie a formato tabular
func construirResultadoTabular(medicionesPorSerie map[string]map[int64]interface{}, timestampsUnicos map[int64]struct{}) tipos.ResultadoConsultaRango {
	// Extraer y ordenar nombres de series alfabéticamente
	seriesOrdenadas := make([]string, 0, len(medicionesPorSerie))
	for path := range medicionesPorSerie {
		seriesOrdenadas = append(seriesOrdenadas, path)
	}
	sort.Strings(seriesOrdenadas)

	// Extraer y ordenar timestamps ascendente
	tiemposOrdenados := make([]int64, 0, len(timestampsUnicos))
	for t := range timestampsUnicos {
		tiemposOrdenados = append(tiemposOrdenados, t)
	}
	sort.Slice(tiemposOrdenados, func(i, j int) bool {
		return tiemposOrdenados[i] < tiemposOrdenados[j]
	})

	// Crear índice de serie -> columna
	indiceColumna := make(map[string]int)
	for i, path := range seriesOrdenadas {
		indiceColumna[path] = i
	}

	// Construir matriz de valores [fila][columna]
	numFilas := len(tiemposOrdenados)
	numColumnas := len(seriesOrdenadas)
	valores := make([][]interface{}, numFilas)

	for i, tiempo := range tiemposOrdenados {
		valores[i] = make([]interface{}, numColumnas)
		for j, path := range seriesOrdenadas {
			if valorMap, existe := medicionesPorSerie[path]; existe {
				if valor, tieneValor := valorMap[tiempo]; tieneValor {
					valores[i][j] = valor
				}
				// Si no tiene valor, queda nil (valor por defecto)
			}
		}
	}

	return tipos.ResultadoConsultaRango{
		Series:  seriesOrdenadas,
		Tiempos: tiemposOrdenados,
		Valores: valores,
	}
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

// ConsultarUltimoPunto obtiene la última medición de cada serie que coincida con el patrón.
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura" o "*/temperatura"
//
// Retorna el último punto de CADA serie en formato columnar.
// Las series sin datos son excluidas del resultado.
func (me *ManagerEdge) ConsultarUltimoPunto(path string) (tipos.ResultadoConsultaPunto, error) {
	// Resolver series (path exacto o patrón wildcard)
	series, err := me.resolverSeries(path)
	if err != nil {
		return tipos.ResultadoConsultaPunto{}, err
	}

	// Recolectar último punto de cada serie
	type puntoSerie struct {
		path   string
		tiempo int64
		valor  interface{}
	}
	var puntos []puntoSerie

	for _, serie := range series {
		medicion, err := me.consultarUltimoPuntoSerie(serie)
		if err != nil {
			continue // Ignorar series sin datos
		}
		puntos = append(puntos, puntoSerie{
			path:   serie.Path,
			tiempo: medicion.Tiempo,
			valor:  medicion.Valor,
		})
	}

	if len(puntos) == 0 {
		return tipos.ResultadoConsultaPunto{}, fmt.Errorf("no hay mediciones para el patrón: %s", path)
	}

	// Ordenar alfabéticamente por path
	sort.Slice(puntos, func(i, j int) bool {
		return puntos[i].path < puntos[j].path
	})

	// Construir resultado columnar
	resultado := tipos.ResultadoConsultaPunto{
		Series:  make([]string, len(puntos)),
		Tiempos: make([]int64, len(puntos)),
		Valores: make([]interface{}, len(puntos)),
	}

	for i, p := range puntos {
		resultado.Series[i] = p.path
		resultado.Tiempos[i] = p.tiempo
		resultado.Valores[i] = p.valor
	}

	return resultado, nil
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
// Retorna un valor agregado por cada serie en formato columnar.
// Las series sin datos en el rango son excluidas del resultado.
func (me *ManagerEdge) ConsultarAgregacion(
	path string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
) (tipos.ResultadoAgregacion, error) {
	// Usar ConsultarRango para obtener datos en formato tabular
	resultado, err := me.ConsultarRango(path, tiempoInicio, tiempoFin)
	if err != nil {
		return tipos.ResultadoAgregacion{}, err
	}

	// Calcular agregación POR COLUMNA (serie)
	seriesConDatos := make([]string, 0, len(resultado.Series))
	valoresFinales := make([]float64, 0, len(resultado.Series))

	for colIdx, nombreSerie := range resultado.Series {
		var valoresColumna []float64

		for filaIdx := range resultado.Tiempos {
			if v := resultado.Valores[filaIdx][colIdx]; v != nil {
				valorFloat, err := convertirAFloat64(v)
				if err != nil {
					// Para COUNT, contar cualquier valor como 1
					if agregacion == tipos.AgregacionCount {
						valorFloat = 1.0
					} else {
						continue // Saltar valores no numéricos para otras agregaciones
					}
				}
				valoresColumna = append(valoresColumna, valorFloat)
			}
		}

		if len(valoresColumna) > 0 {
			valor, err := CalcularAgregacionSimple(valoresColumna, agregacion)
			if err == nil {
				seriesConDatos = append(seriesConDatos, nombreSerie)
				valoresFinales = append(valoresFinales, valor)
			}
		}
	}

	if len(seriesConDatos) == 0 {
		return tipos.ResultadoAgregacion{}, fmt.Errorf("no hay datos en el rango especificado para: %s", path)
	}

	return tipos.ResultadoAgregacion{
		Series:  seriesConDatos, // Ya ordenadas alfabéticamente por ConsultarRango
		Valores: valoresFinales,
	}, nil
}

// ConsultarAgregacionTemporal calcula agregaciones agrupadas por intervalos de tiempo (downsampling).
// Retorna una matriz donde cada columna es una serie y cada fila es un bucket temporal.
// Los valores faltantes (bucket sin datos para una serie) se representan como math.NaN().
//
// El parámetro path puede ser:
//   - Path exacto: "sensor_01/temperatura"
//   - Patrón con wildcard: "sensor_*/temperatura"
//
// Ejemplo: ConsultarAgregacionTemporal("sensor_01/temp", inicio, fin, AgregacionPromedio, time.Hour)
// retorna el promedio por cada hora en el rango para cada serie.
func (me *ManagerEdge) ConsultarAgregacionTemporal(
	path string,
	tiempoInicio, tiempoFin time.Time,
	agregacion tipos.TipoAgregacion,
	intervalo time.Duration,
) (tipos.ResultadoAgregacionTemporal, error) {
	if intervalo <= 0 {
		return tipos.ResultadoAgregacionTemporal{}, fmt.Errorf("el intervalo debe ser mayor a cero")
	}

	// Usar ConsultarRango para obtener datos en formato tabular
	resultado, err := me.ConsultarRango(path, tiempoInicio, tiempoFin)
	if err != nil {
		return tipos.ResultadoAgregacionTemporal{}, err
	}

	// Si no hay datos, retornar error
	if len(resultado.Series) == 0 || len(resultado.Tiempos) == 0 {
		return tipos.ResultadoAgregacionTemporal{}, fmt.Errorf("no hay datos en el rango especificado para: %s", path)
	}

	// Generar buckets temporales
	buckets := generarBuckets(tiempoInicio.UnixNano(), tiempoFin.UnixNano(), intervalo.Nanoseconds())
	numBuckets := len(buckets)
	numSeries := len(resultado.Series)

	// Inicializar acumuladores para cada [bucket][serie]
	// Usamos slices para acumular valores antes de calcular la agregación
	acumuladores := make([][][]float64, numBuckets)
	for b := 0; b < numBuckets; b++ {
		acumuladores[b] = make([][]float64, numSeries)
		for s := 0; s < numSeries; s++ {
			acumuladores[b][s] = make([]float64, 0)
		}
	}

	// Distribuir valores en acumuladores
	intervaloNano := intervalo.Nanoseconds()
	tiempoInicioNano := tiempoInicio.UnixNano()

	for filaIdx, tiempo := range resultado.Tiempos {
		bucketIdx := calcularBucketIdx(tiempo, tiempoInicioNano, intervaloNano, numBuckets)
		if bucketIdx < 0 || bucketIdx >= numBuckets {
			continue
		}

		for colIdx := 0; colIdx < numSeries; colIdx++ {
			valor := resultado.Valores[filaIdx][colIdx]
			if valor == nil {
				continue
			}

			valorFloat, err := convertirAFloat64(valor)
			if err != nil {
				if agregacion == tipos.AgregacionCount {
					valorFloat = 1.0
				} else {
					continue
				}
			}
			acumuladores[bucketIdx][colIdx] = append(acumuladores[bucketIdx][colIdx], valorFloat)
		}
	}

	// Calcular agregación y construir matriz de resultados
	valores := make([][]float64, numBuckets)
	for b := 0; b < numBuckets; b++ {
		valores[b] = make([]float64, numSeries)
		for s := 0; s < numSeries; s++ {
			if len(acumuladores[b][s]) == 0 {
				valores[b][s] = math.NaN()
			} else {
				valorAgregado, err := CalcularAgregacionSimple(acumuladores[b][s], agregacion)
				if err != nil {
					valores[b][s] = math.NaN()
				} else {
					valores[b][s] = valorAgregado
				}
			}
		}
	}

	return tipos.ResultadoAgregacionTemporal{
		Series:  resultado.Series, // Ya ordenadas alfabéticamente por ConsultarRango
		Tiempos: buckets,
		Valores: valores,
	}, nil
}

// generarBuckets genera los timestamps de inicio de cada bucket temporal
func generarBuckets(tiempoInicio, tiempoFin, intervalo int64) []int64 {
	var buckets []int64
	for t := tiempoInicio; t < tiempoFin; t += intervalo {
		buckets = append(buckets, t)
	}
	return buckets
}

// calcularBucketIdx calcula el índice del bucket para un timestamp dado
func calcularBucketIdx(tiempo, tiempoInicio, intervalo int64, numBuckets int) int {
	if tiempo < tiempoInicio {
		return -1
	}
	idx := int((tiempo - tiempoInicio) / intervalo)
	if idx >= numBuckets {
		return numBuckets - 1 // Último bucket captura valores hasta tiempoFin
	}
	return idx
}
