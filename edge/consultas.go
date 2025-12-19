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

// ConsultarRango consulta mediciones de una serie dentro de un rango de tiempo
// y descomprime los datos antes de retornarlos
func (me *ManagerEdge) ConsultarRango(nombreSerie string, tiempoInicio, tiempoFin time.Time) ([]tipos.Medicion, error) {
	// Convertir los tiempos a Unix timestamp (en nanosegundos)
	tiempoInicioUnix := tiempoInicio.UnixNano()
	tiempoFinUnix := tiempoFin.UnixNano()
	// Obtener configuración de la serie desde cache
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return nil, fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

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
	if bufferInterface, ok := me.buffers.Load(nombreSerie); ok {
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

// ConsultarUltimoPunto obtiene la última medición registrada para una serie
func (me *ManagerEdge) ConsultarUltimoPunto(nombreSerie string) (tipos.Medicion, error) {
	// Primero revisar el buffer en memoria
	if bufferInterface, ok := me.buffers.Load(nombreSerie); ok {
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

	// Si no hay datos en buffer, consultar la base de datos
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("serie no encontrada: %s", nombreSerie)
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
		return tipos.Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", nombreSerie)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("error al descomprimir último bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return tipos.Medicion{}, fmt.Errorf("bloque vacío para serie: %s", nombreSerie)
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

// ConsultarPrimerPunto obtiene la primera medición registrada para una serie
func (me *ManagerEdge) ConsultarPrimerPunto(nombreSerie string) (tipos.Medicion, error) {
	serie, err := me.ObtenerSeries(nombreSerie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("serie no encontrada: %s", nombreSerie)
	}

	// Buscar el primer bloque para esta serie
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

	// Ir al primer elemento
	if !iter.First() {
		return tipos.Medicion{}, fmt.Errorf("no hay mediciones para la serie: %s", nombreSerie)
	}

	datosComprimidos := make([]byte, len(iter.Value()))
	copy(datosComprimidos, iter.Value())

	mediciones, err := me.descomprimirBloque(datosComprimidos, serie)
	if err != nil {
		return tipos.Medicion{}, fmt.Errorf("error al descomprimir primer bloque: %v", err)
	}

	if len(mediciones) == 0 {
		return tipos.Medicion{}, fmt.Errorf("bloque vacío para serie: %s", nombreSerie)
	}

	// Encontrar la medición más antigua en el bloque
	primeraMedicion := mediciones[0]
	for _, m := range mediciones[1:] {
		if m.Tiempo < primeraMedicion.Tiempo {
			primeraMedicion = m
		}
	}

	return primeraMedicion, nil
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
