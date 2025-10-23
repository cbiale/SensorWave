package edge

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
)

type TipoOperador string

const (
	OperadorMayorIgual TipoOperador = ">="
	OperadorMenorIgual TipoOperador = "<="
	OperadorIgual      TipoOperador = "=="
	OperadorDistinto   TipoOperador = "!="
	OperadorMayor      TipoOperador = ">"
	OperadorMenor      TipoOperador = "<"
)

type TipoAgregacion string

const (
	AgregacionPromedio TipoAgregacion = "promedio"
	AgregacionMaximo   TipoAgregacion = "maximo"
	AgregacionMinimo   TipoAgregacion = "minimo"
	AgregacionSuma     TipoAgregacion = "suma"
	AgregacionCount    TipoAgregacion = "count"
)

type TipoLogica string

const (
	LogicaAND TipoLogica = "AND"
	LogicaOR  TipoLogica = "OR"
)

type Condicion struct {
	Serie       string
	SeriesGrupo []string
	PathPattern string            // Patrón de path: "dispositivo_001/*"
	TagsFilter  map[string]string // Filtro por tags: {"ubicacion": "sala1"}
	Agregacion  TipoAgregacion
	Operador    TipoOperador
	Valor       float64
	VentanaT    time.Duration
}

type Accion struct {
	Tipo    string
	Destino string
	Params  map[string]string
}

type Regla struct {
	ID          string
	Nombre      string
	Condiciones []Condicion
	Acciones    []Accion
	Logica      TipoLogica
	Activa      bool
	UltimaEval  time.Time
}

type DatoTemporal struct {
	Timestamp time.Time
	Valor     float64
}

type EjecutorAccion func(accion Accion, regla *Regla, valores map[string]float64) error

type MotorReglas struct {
	reglas         map[string]*Regla
	datos          map[string][]DatoTemporal
	ejecutores     map[string]EjecutorAccion
	habilitado     bool
	maxDatosCache  int
	tiempoLimpieza time.Duration
	mu             sync.RWMutex
	logger         *log.Logger
	manager        *ManagerEdge // Referencia al manager padre (para acceso a datos)
	db             *pebble.DB   // Conexión a PebbleDB para persistencia de reglas
}

func nuevoMotorReglasIntegrado(manager *ManagerEdge, db *pebble.DB) *MotorReglas {
	motor := &MotorReglas{
		reglas:         make(map[string]*Regla),
		datos:          make(map[string][]DatoTemporal),
		ejecutores:     make(map[string]EjecutorAccion),
		habilitado:     true,
		maxDatosCache:  1000,
		tiempoLimpieza: 5 * time.Minute,
		logger:         log.New(log.Writer(), "[MotorReglas] ", log.LstdFlags|log.Lshortfile),
		manager:        manager,
		db:             db,
	}

	motor.registrarEjecutoresPorDefecto()
	return motor
}

func (mr *MotorReglas) registrarEjecutoresPorDefecto() {
	mr.ejecutores["log"] = func(accion Accion, regla *Regla, valores map[string]float64) error {
		mr.logger.Printf("Regla '%s' activada - Acción: %s, Destino: %s, Valores: %v",
			regla.ID, accion.Tipo, accion.Destino, valores)
		return nil
	}

	mr.ejecutores["enviar_alerta"] = func(accion Accion, regla *Regla, valores map[string]float64) error {
		mr.logger.Printf("ALERTA: Regla '%s' - %s. Valores: %v",
			regla.ID, accion.Destino, valores)
		return nil
	}

	mr.ejecutores["activar_actuador"] = func(accion Accion, regla *Regla, valores map[string]float64) error {
		mr.logger.Printf("ACTUADOR: Activando %s por regla '%s'. Valores: %v",
			accion.Destino, regla.ID, valores)
		return nil
	}
}

func (mr *MotorReglas) ProcesarDato(serie string, valor float64, timestamp time.Time) error {
	if !mr.habilitado {
		return nil
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	nuevoDato := DatoTemporal{
		Timestamp: timestamp,
		Valor:     valor,
	}

	if _, exists := mr.datos[serie]; !exists {
		mr.datos[serie] = make([]DatoTemporal, 0, mr.maxDatosCache)
	}

	mr.datos[serie] = append(mr.datos[serie], nuevoDato)

	if len(mr.datos[serie]) > mr.maxDatosCache {
		mr.datos[serie] = mr.datos[serie][len(mr.datos[serie])-mr.maxDatosCache:]
	}

	return mr.evaluarReglas(timestamp)
}

func (mr *MotorReglas) evaluarReglas(timestamp time.Time) error {

	for _, regla := range mr.reglas {
		if !regla.Activa {
			continue
		}

		if mr.evaluarCondicionesRegla(regla, timestamp) {
			if err := mr.ejecutarAcciones(regla, timestamp); err != nil {
				mr.logger.Printf("Error ejecutando acciones de regla '%s': %v", regla.ID, err)
			}
		}

		regla.UltimaEval = timestamp
	}

	return nil
}

func (mr *MotorReglas) evaluarCondicionesRegla(regla *Regla, timestamp time.Time) bool {
	if len(regla.Condiciones) == 0 {
		return false
	}

	resultados := make([]bool, len(regla.Condiciones))

	for i, condicion := range regla.Condiciones {
		resultados[i] = mr.evaluarCondicion(&condicion, timestamp)
	}

	if regla.Logica == LogicaOR {
		for _, resultado := range resultados {
			if resultado {
				return true
			}
		}
		return false
	}

	for _, resultado := range resultados {
		if !resultado {
			return false
		}
	}
	return true
}

// calcularAgregacionSimple calcula una agregación sobre un slice de valores
func calcularAgregacionSimple(valores []float64, agregacion TipoAgregacion) (float64, error) {
	if len(valores) == 0 {
		return 0, fmt.Errorf("no hay valores para agregar")
	}

	switch agregacion {
	case AgregacionPromedio:
		suma := 0.0
		for _, v := range valores {
			suma += v
		}
		return suma / float64(len(valores)), nil

	case AgregacionMaximo:
		max := valores[0]
		for _, v := range valores[1:] {
			if v > max {
				max = v
			}
		}
		return max, nil

	case AgregacionMinimo:
		min := valores[0]
		for _, v := range valores[1:] {
			if v < min {
				min = v
			}
		}
		return min, nil

	case AgregacionSuma:
		suma := 0.0
		for _, v := range valores {
			suma += v
		}
		return suma, nil

	case AgregacionCount:
		return float64(len(valores)), nil

	default:
		return 0, fmt.Errorf("tipo de agregación no soportado: %s", agregacion)
	}
}

func (mr *MotorReglas) evaluarCondicion(condicion *Condicion, timestamp time.Time) bool {
	tiempoInicio := timestamp.Add(-condicion.VentanaT)

	var valorEvaluacion float64
	var err error

	if condicion.PathPattern != "" || len(condicion.TagsFilter) > 0 {
		seriesResueltas := mr.resolverSeriesPorCondicion(condicion)
		if len(seriesResueltas) == 0 {
			return false
		}
		valorEvaluacion, err = mr.calcularAgregacion(seriesResueltas, condicion.Agregacion, tiempoInicio, timestamp)
	} else if len(condicion.SeriesGrupo) > 0 {
		valorEvaluacion, err = mr.calcularAgregacion(condicion.SeriesGrupo, condicion.Agregacion, tiempoInicio, timestamp)
	} else {
		valorEvaluacion, err = mr.obtenerValorSerie(condicion.Serie, tiempoInicio, timestamp)
	}

	if err != nil {
		return false
	}

	return mr.aplicarOperador(valorEvaluacion, condicion.Operador, condicion.Valor)
}

func (mr *MotorReglas) calcularAgregacion(series []string, agregacion TipoAgregacion, tiempoInicio, tiempoFin time.Time) (float64, error) {
	var valoresPorSerie []float64

	// Calcular agregación POR CADA serie
	for _, serie := range series {
		datosValidos := mr.obtenerDatosEnVentana(serie, tiempoInicio, tiempoFin)
		if len(datosValidos) == 0 {
			continue
		}

		// Extraer valores de los datos temporales
		valores := make([]float64, len(datosValidos))
		for i, dato := range datosValidos {
			valores[i] = dato.Valor
		}

		// Usar helper para calcular agregación de esta serie
		valorSerie, err := calcularAgregacionSimple(valores, agregacion)
		if err != nil {
			continue
		}

		valoresPorSerie = append(valoresPorSerie, valorSerie)
	}

	if len(valoresPorSerie) == 0 {
		return 0, fmt.Errorf("no hay datos disponibles para la agregación")
	}

	// Usar helper nuevamente para agregar entre series
	return calcularAgregacionSimple(valoresPorSerie, agregacion)
}

func (mr *MotorReglas) obtenerValorSerie(serie string, tiempoInicio, tiempoFin time.Time) (float64, error) {
	datosValidos := mr.obtenerDatosEnVentana(serie, tiempoInicio, tiempoFin)

	if len(datosValidos) == 0 {
		return 0, fmt.Errorf("no hay datos disponibles para la serie %s", serie)
	}

	return datosValidos[len(datosValidos)-1].Valor, nil
}

func (mr *MotorReglas) obtenerDatosEnVentana(serie string, tiempoInicio, tiempoFin time.Time) []DatoTemporal {
	var datosValidos []DatoTemporal

	// Si tenemos acceso al manager, usar datos de la base de datos
	if mr.manager != nil {
		mediciones, err := mr.manager.ConsultarRango(serie, tiempoInicio, tiempoFin)
		if err == nil {
			// Convertir Medicion a DatoTemporal
			for _, medicion := range mediciones {
				if valor, ok := medicion.Valor.(float64); ok {
					datosValidos = append(datosValidos, DatoTemporal{
						Timestamp: time.Unix(0, medicion.Tiempo),
						Valor:     valor,
					})
				}
			}
			return datosValidos
		}
		// Si falla la consulta a BD, continúa con cache local
	}

	// Fallback al cache local
	datos, exists := mr.datos[serie]
	if !exists {
		return nil
	}

	for _, dato := range datos {
		if dato.Timestamp.After(tiempoInicio) && dato.Timestamp.Before(tiempoFin) || dato.Timestamp.Equal(tiempoFin) {
			datosValidos = append(datosValidos, dato)
		}
	}

	sort.Slice(datosValidos, func(i, j int) bool {
		return datosValidos[i].Timestamp.Before(datosValidos[j].Timestamp)
	})

	return datosValidos
}

func (mr *MotorReglas) aplicarOperador(valor1 float64, operador TipoOperador, valor2 float64) bool {
	const epsilon = 1e-9

	switch operador {
	case OperadorMayorIgual:
		return valor1 >= valor2
	case OperadorMenorIgual:
		return valor1 <= valor2
	case OperadorIgual:
		return math.Abs(valor1-valor2) < epsilon
	case OperadorDistinto:
		return math.Abs(valor1-valor2) >= epsilon
	case OperadorMayor:
		return valor1 > valor2
	case OperadorMenor:
		return valor1 < valor2
	default:
		return false
	}
}

func (mr *MotorReglas) ejecutarAcciones(regla *Regla, timestamp time.Time) error {
	valores := make(map[string]float64)

	for _, condicion := range regla.Condiciones {
		if len(condicion.SeriesGrupo) > 0 {
			for _, serie := range condicion.SeriesGrupo {
				if valor, err := mr.obtenerValorSerie(serie, timestamp.Add(-condicion.VentanaT), timestamp); err == nil {
					valores[serie] = valor
				}
			}
		} else {
			if valor, err := mr.obtenerValorSerie(condicion.Serie, timestamp.Add(-condicion.VentanaT), timestamp); err == nil {
				valores[condicion.Serie] = valor
			}
		}
	}

	for _, accion := range regla.Acciones {
		ejecutor, exists := mr.ejecutores[accion.Tipo]
		if !exists {
			mr.logger.Printf("Ejecutor no encontrado para tipo de acción: %s", accion.Tipo)
			continue
		}

		if err := ejecutor(accion, regla, valores); err != nil {
			return fmt.Errorf("error ejecutando acción %s: %v", accion.Tipo, err)
		}
	}

	return nil
}

func (mr *MotorReglas) RegistrarEjecutor(tipoAccion string, ejecutor EjecutorAccion) error {
	if tipoAccion == "" {
		return fmt.Errorf("tipo de acción no puede estar vacío")
	}
	if ejecutor == nil {
		return fmt.Errorf("ejecutor no puede ser nil")
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.ejecutores[tipoAccion] = ejecutor
	mr.logger.Printf("Ejecutor registrado para tipo de acción: %s", tipoAccion)
	return nil
}

func (mr *MotorReglas) Habilitar(habilitado bool) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.habilitado = habilitado
	estado := "deshabilitado"
	if habilitado {
		estado = "habilitado"
	}
	mr.logger.Printf("Motor de reglas %s", estado)
}

func (mr *MotorReglas) LimpiarDatosAntiguos(tiempoRetencion time.Duration) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	tiempoLimite := time.Now().Add(-tiempoRetencion)
	datosEliminados := 0

	for serie, datos := range mr.datos {
		var datosFiltrados []DatoTemporal
		for _, dato := range datos {
			if dato.Timestamp.After(tiempoLimite) {
				datosFiltrados = append(datosFiltrados, dato)
			} else {
				datosEliminados++
			}
		}
		mr.datos[serie] = datosFiltrados

		if len(mr.datos[serie]) == 0 {
			delete(mr.datos, serie)
		}
	}

	mr.logger.Printf("Limpieza completada: %d datos eliminados", datosEliminados)
}

func (mr *MotorReglas) contarDatosEnCache() int {
	total := 0
	for _, datos := range mr.datos {
		total += len(datos)
	}
	return total
}

func (mr *MotorReglas) validarRegla(regla *Regla) error {
	if regla.ID == "" {
		return fmt.Errorf("ID de regla no puede estar vacío")
	}

	if len(regla.Condiciones) == 0 {
		return fmt.Errorf("regla debe tener al menos una condición")
	}

	if len(regla.Acciones) == 0 {
		return fmt.Errorf("regla debe tener al menos una acción")
	}

	for i, condicion := range regla.Condiciones {
		if err := mr.validarCondicion(&condicion); err != nil {
			return fmt.Errorf("condición %d inválida: %v", i, err)
		}
	}

	for i, accion := range regla.Acciones {
		if err := mr.validarAccion(&accion); err != nil {
			return fmt.Errorf("acción %d inválida: %v", i, err)
		}
	}

	if regla.Logica != LogicaAND && regla.Logica != LogicaOR {
		regla.Logica = LogicaAND
	}

	return nil
}

func (mr *MotorReglas) validarCondicion(condicion *Condicion) error {
	tieneSerieIndividual := condicion.Serie != ""
	tieneGrupo := len(condicion.SeriesGrupo) > 0
	tienePathPattern := condicion.PathPattern != ""
	tieneTagsFilter := len(condicion.TagsFilter) > 0

	if !tieneSerieIndividual && !tieneGrupo && !tienePathPattern && !tieneTagsFilter {
		return fmt.Errorf("debe especificar una serie, grupo de series, PathPattern o TagsFilter")
	}

	if tieneSerieIndividual && tieneGrupo {
		return fmt.Errorf("no se puede especificar tanto serie individual como grupo de series")
	}

	if (tienePathPattern || tieneTagsFilter) && (tieneSerieIndividual || tieneGrupo) {
		return fmt.Errorf("PathPattern/TagsFilter no se puede combinar con Serie/SeriesGrupo")
	}

	if len(condicion.SeriesGrupo) > 0 && condicion.Agregacion == "" {
		return fmt.Errorf("debe especificar tipo de agregación para grupo de series")
	}

	if (tienePathPattern || tieneTagsFilter) && condicion.Agregacion == "" {
		return fmt.Errorf("debe especificar tipo de agregación para PathPattern/TagsFilter")
	}

	if condicion.VentanaT <= 0 {
		return fmt.Errorf("ventana temporal debe ser mayor a cero")
	}

	operadoresValidos := []TipoOperador{OperadorMayorIgual, OperadorMenorIgual, OperadorIgual, OperadorDistinto, OperadorMayor, OperadorMenor}
	operadorValido := false
	for _, op := range operadoresValidos {
		if condicion.Operador == op {
			operadorValido = true
			break
		}
	}
	if !operadorValido {
		return fmt.Errorf("operador inválido: %s", condicion.Operador)
	}

	if len(condicion.SeriesGrupo) > 0 {
		agregacionesValidas := []TipoAgregacion{AgregacionPromedio, AgregacionMaximo, AgregacionMinimo, AgregacionSuma, AgregacionCount}
		agregacionValida := false
		for _, agg := range agregacionesValidas {
			if condicion.Agregacion == agg {
				agregacionValida = true
				break
			}
		}
		if !agregacionValida {
			return fmt.Errorf("agregación inválida: %s", condicion.Agregacion)
		}
	}

	return nil
}

func (mr *MotorReglas) validarAccion(accion *Accion) error {
	if accion.Tipo == "" {
		return fmt.Errorf("tipo de acción no puede estar vacío")
	}

	if accion.Destino == "" {
		return fmt.Errorf("destino de acción no puede estar vacío")
	}

	return nil
}

func (mr *MotorReglas) IniciarLimpiezaAutomatica() {
	go func() {
		ticker := time.NewTicker(mr.tiempoLimpieza)
		defer ticker.Stop()

		for range ticker.C {
			mr.LimpiarDatosAntiguos(mr.tiempoLimpieza * 2)
		}
	}()
}

func (mr *MotorReglas) ListarReglas() map[string]*Regla {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	copia := make(map[string]*Regla)
	for id, regla := range mr.reglas {
		copia[id] = regla
	}
	return copia
}

func (mr *MotorReglas) ObtenerRegla(id string) (*Regla, error) {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	regla, exists := mr.reglas[id]
	if !exists {
		return nil, fmt.Errorf("regla '%s' no encontrada", id)
	}

	return regla, nil
}

// resolverSeriesPorCondicion resuelve las series que coinciden con PathPattern o TagsFilter
func (mr *MotorReglas) resolverSeriesPorCondicion(condicion *Condicion) []string {
	var seriesResueltas []string

	if mr.manager == nil {
		return seriesResueltas
	}

	// Si hay PathPattern, usar ese filtro
	if condicion.PathPattern != "" {
		series, err := mr.manager.ListarSeriesPorPath(condicion.PathPattern)
		if err == nil {
			for _, serie := range series {
				seriesResueltas = append(seriesResueltas, serie.Path)
			}
		}
	}

	// Si hay TagsFilter, filtrar por tags
	if len(condicion.TagsFilter) > 0 {
		series, err := mr.manager.ListarSeriesPorTags(condicion.TagsFilter)
		if err == nil {
			for _, serie := range series {
				seriesResueltas = append(seriesResueltas, serie.Path)
			}
		}
	}

	return seriesResueltas
}

// AgregarReglaEnMemoria agrega una regla al motor sin persistencia
func (mr *MotorReglas) AgregarReglaEnMemoria(regla *Regla) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.reglas[regla.ID] = regla
	return nil
}

// EliminarReglaEnMemoria elimina una regla del motor sin persistencia
func (mr *MotorReglas) EliminarReglaEnMemoria(id string) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if _, exists := mr.reglas[id]; !exists {
		return fmt.Errorf("regla '%s' no encontrada", id)
	}

	delete(mr.reglas, id)
	return nil
}

// ActualizarReglaEnMemoria actualiza una regla en el motor sin persistencia
func (mr *MotorReglas) ActualizarReglaEnMemoria(regla *Regla) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if _, exists := mr.reglas[regla.ID]; !exists {
		return fmt.Errorf("regla '%s' no encontrada", regla.ID)
	}

	mr.reglas[regla.ID] = regla
	return nil
}

func generarClaveRegla(id string) []byte {
	return []byte("reglas/" + id)
}

func serializarRegla(regla *Regla) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	err := encoder.Encode(regla)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func deserializarRegla(data []byte) (*Regla, error) {
	var regla Regla
	buffer := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(buffer)
	err := decoder.Decode(&regla)
	if err != nil {
		return nil, err
	}
	return &regla, nil
}

func (mr *MotorReglas) cargarReglasExistentes() error {
	if mr.db == nil {
		return nil
	}

	iter, err := mr.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("reglas/"),
		UpperBound: []byte("reglas0"),
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		regla, err := deserializarRegla(iter.Value())
		if err != nil {
			continue
		}

		mr.AgregarReglaEnMemoria(regla)
	}

	return iter.Error()
}

func (mr *MotorReglas) AgregarRegla(regla *Regla) error {
	if regla == nil {
		return fmt.Errorf("regla no puede ser nil")
	}

	if err := mr.validarRegla(regla); err != nil {
		return fmt.Errorf("regla inválida: %v", err)
	}

	if mr.db != nil {
		key := generarClaveRegla(regla.ID)
		reglaBytes, err := serializarRegla(regla)
		if err != nil {
			return fmt.Errorf("error al serializar regla: %v", err)
		}

		err = mr.db.Set(key, reglaBytes, pebble.Sync)
		if err != nil {
			return fmt.Errorf("error al guardar regla: %v", err)
		}
	}

	regla.Activa = true
	if err := mr.AgregarReglaEnMemoria(regla); err != nil {
		return err
	}

	mr.logger.Printf("Regla '%s' agregada exitosamente", regla.ID)
	return nil
}

func (mr *MotorReglas) EliminarRegla(id string) error {
	if mr.db != nil {
		key := generarClaveRegla(id)
		err := mr.db.Delete(key, pebble.Sync)
		if err != nil {
			return fmt.Errorf("error al eliminar regla de DB: %v", err)
		}
	}

	if err := mr.EliminarReglaEnMemoria(id); err != nil {
		return err
	}

	mr.logger.Printf("Regla '%s' eliminada", id)
	return nil
}

func (mr *MotorReglas) ActualizarRegla(regla *Regla) error {
	if regla == nil {
		return fmt.Errorf("regla no puede ser nil")
	}

	if err := mr.validarRegla(regla); err != nil {
		return fmt.Errorf("regla inválida: %v", err)
	}

	if mr.db != nil {
		key := generarClaveRegla(regla.ID)
		reglaBytes, err := serializarRegla(regla)
		if err != nil {
			return fmt.Errorf("error al serializar regla: %v", err)
		}

		err = mr.db.Set(key, reglaBytes, pebble.Sync)
		if err != nil {
			return fmt.Errorf("error al actualizar regla en DB: %v", err)
		}
	}

	if err := mr.ActualizarReglaEnMemoria(regla); err != nil {
		return err
	}

	mr.logger.Printf("Regla '%s' actualizada", regla.ID)
	return nil
}
