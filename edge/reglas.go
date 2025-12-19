package edge

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cbiale/sensorwave/tipos"
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
	FiltroValor interface{} // Filtro pre-agregación (nil = sin filtro, solo soportado con AgregacionCount)
	Operador    TipoOperador
	Valor       interface{} // Valor umbral para comparación (soporta bool, int64, float64, string)
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
	Valor     interface{} // Soporta Boolean, Integer (int64), Real (float64), Text (string)
}

type EjecutorAccion func(accion Accion, regla *Regla, valores map[string]interface{}) error

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
	mr.ejecutores["log"] = func(accion Accion, regla *Regla, valores map[string]interface{}) error {
		mr.logger.Printf("Regla '%s' activada - Acción: %s, Destino: %s, Valores: %v",
			regla.ID, accion.Tipo, accion.Destino, valores)
		return nil
	}

	mr.ejecutores["enviar_alerta"] = func(accion Accion, regla *Regla, valores map[string]interface{}) error {
		mr.logger.Printf("ALERTA: Regla '%s' - %s. Valores: %v",
			regla.ID, accion.Destino, valores)
		return nil
	}

	mr.ejecutores["activar_actuador"] = func(accion Accion, regla *Regla, valores map[string]interface{}) error {
		mr.logger.Printf("ACTUADOR: Activando %s por regla '%s'. Valores: %v",
			accion.Destino, regla.ID, valores)
		return nil
	}
}

func (mr *MotorReglas) ProcesarDato(serie string, valor interface{}, timestamp time.Time) error {
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

	var valorEvaluacion interface{}
	var err error

	if condicion.PathPattern != "" || len(condicion.TagsFilter) > 0 {
		seriesResueltas := mr.resolverSeriesPorCondicion(condicion)
		if len(seriesResueltas) == 0 {
			return false
		}
		// Para agregaciones, siempre retorna float64
		valorEvaluacion, err = mr.calcularAgregacion(seriesResueltas, condicion.Agregacion, tiempoInicio, timestamp, condicion.FiltroValor)
	} else if len(condicion.SeriesGrupo) > 0 {
		// Para agregaciones, siempre retorna float64
		valorEvaluacion, err = mr.calcularAgregacion(condicion.SeriesGrupo, condicion.Agregacion, tiempoInicio, timestamp, condicion.FiltroValor)
	} else {
		// Para serie individual, puede retornar cualquier tipo
		valorEvaluacion, err = mr.obtenerValorSerie(condicion.Serie, tiempoInicio, timestamp)
	}

	if err != nil {
		return false
	}

	return mr.aplicarOperador(valorEvaluacion, condicion.Operador, condicion.Valor)
}

// convertirAFloat64 convierte un valor interface{} a float64 para agregaciones numéricas
func convertirAFloat64(valor interface{}) (float64, error) {
	switch v := valor.(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("tipo %T no se puede convertir a float64", valor)
	}
}

// coincideFiltro verifica si un valor coincide con el filtro
func coincideFiltro(valor interface{}, filtro interface{}) bool {
	// Mismo tipo: comparar directamente
	switch v := valor.(type) {
	case bool:
		f, ok := filtro.(bool)
		if !ok {
			return false
		}
		return v == f

	case string:
		f, ok := filtro.(string)
		if !ok {
			return false
		}
		// Case-insensitive para strings
		return strings.EqualFold(v, f)

	case int64:
		// Filtro puede ser int64 o float64
		switch f := filtro.(type) {
		case int64:
			return v == f
		case float64:
			return float64(v) == f
		default:
			return false
		}

	case float64:
		// Filtro puede ser int64 o float64
		const epsilon = 1e-9
		switch f := filtro.(type) {
		case int64:
			return math.Abs(v-float64(f)) < epsilon
		case float64:
			return math.Abs(v-f) < epsilon
		default:
			return false
		}

	default:
		return false
	}
}

func (mr *MotorReglas) calcularAgregacion(series []string, agregacion TipoAgregacion, tiempoInicio, tiempoFin time.Time, filtroValor interface{}) (float64, error) {
	var valoresPorSerie []float64

	// Calcular agregación POR CADA serie
	for _, serie := range series {
		datosValidos := mr.obtenerDatosEnVentana(serie, tiempoInicio, tiempoFin)
		if len(datosValidos) == 0 {
			continue
		}

		// Extraer valores de los datos temporales y convertir a float64
		valores := make([]float64, 0, len(datosValidos))
		for _, dato := range datosValidos {
			// Aplicar filtro si existe
			if filtroValor != nil {
				if !coincideFiltro(dato.Valor, filtroValor) {
					continue // Saltar este valor
				}
			}

			// Convertir a float64 para agregación numérica
			valorFloat, err := convertirAFloat64(dato.Valor)
			if err != nil {
				// Si no se puede convertir (ej: string, boolean) y estamos haciendo count, usar 1.0
				if agregacion == AgregacionCount {
					valorFloat = 1.0
				} else {
					continue // Saltar valores no numéricos para otras agregaciones
				}
			}
			valores = append(valores, valorFloat)
		}

		if len(valores) == 0 {
			continue
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

func (mr *MotorReglas) obtenerValorSerie(serie string, tiempoInicio, tiempoFin time.Time) (interface{}, error) {
	datosValidos := mr.obtenerDatosEnVentana(serie, tiempoInicio, tiempoFin)

	if len(datosValidos) == 0 {
		return nil, fmt.Errorf("no hay datos disponibles para la serie %s", serie)
	}

	return datosValidos[len(datosValidos)-1].Valor, nil
}

func (mr *MotorReglas) obtenerDatosEnVentana(serie string, tiempoInicio, tiempoFin time.Time) []DatoTemporal {
	var datosValidos []DatoTemporal

	// Primero verificar cache local (más rápido y tiene datos recientes)
	datos, existsInCache := mr.datos[serie]
	if existsInCache {
		for _, dato := range datos {
			if dato.Timestamp.After(tiempoInicio) && dato.Timestamp.Before(tiempoFin) || dato.Timestamp.Equal(tiempoFin) {
				datosValidos = append(datosValidos, dato)
			}
		}

		// Si encontramos datos en cache, retornarlos
		if len(datosValidos) > 0 {
			sort.Slice(datosValidos, func(i, j int) bool {
				return datosValidos[i].Timestamp.Before(datosValidos[j].Timestamp)
			})
			return datosValidos
		}
	}

	// Si no hay datos en cache, consultar la base de datos
	if mr.manager != nil {
		mediciones, err := mr.manager.ConsultarRango(serie, tiempoInicio, tiempoFin)
		if err == nil {
			// Convertir Medicion a DatoTemporal (ahora acepta todos los tipos)
			for _, medicion := range mediciones {
				datosValidos = append(datosValidos, DatoTemporal{
					Timestamp: time.Unix(0, medicion.Tiempo),
					Valor:     medicion.Valor,
				})
			}
			return datosValidos
		}
	}

	return nil
}

func (mr *MotorReglas) aplicarOperador(valor1 interface{}, operador TipoOperador, valor2 interface{}) bool {
	const epsilon = 1e-9

	// Caso 1: Comparación Boolean
	if v1, ok1 := valor1.(bool); ok1 {
		if v2, ok2 := valor2.(bool); ok2 {
			switch operador {
			case OperadorIgual:
				return v1 == v2
			case OperadorDistinto:
				return v1 != v2
			default:
				mr.logger.Printf("Operador %s no soportado para boolean", operador)
				return false
			}
		}
		// Tipos incompatibles
		mr.logger.Printf("No se puede comparar bool con %T", valor2)
		return false
	}

	// Caso 2: Comparación String (case-insensitive)
	if v1, ok1 := valor1.(string); ok1 {
		if v2, ok2 := valor2.(string); ok2 {
			v1Lower := strings.ToLower(v1)
			v2Lower := strings.ToLower(v2)

			switch operador {
			case OperadorIgual:
				return v1Lower == v2Lower
			case OperadorDistinto:
				return v1Lower != v2Lower
			default:
				mr.logger.Printf("Operador %s no soportado para string", operador)
				return false
			}
		}
		// Tipos incompatibles
		mr.logger.Printf("No se puede comparar string con %T", valor2)
		return false
	}

	// Caso 3: Comparación Numérica (int64 y float64 son intercambiables)
	var v1Float, v2Float float64
	var ok1, ok2 bool

	// Convertir valor1 a float64
	switch v := valor1.(type) {
	case int64:
		v1Float = float64(v)
		ok1 = true
	case float64:
		v1Float = v
		ok1 = true
	}

	// Convertir valor2 a float64
	switch v := valor2.(type) {
	case int64:
		v2Float = float64(v)
		ok2 = true
	case float64:
		v2Float = v
		ok2 = true
	}

	if ok1 && ok2 {
		// Comparación numérica
		switch operador {
		case OperadorMayorIgual:
			return v1Float >= v2Float
		case OperadorMenorIgual:
			return v1Float <= v2Float
		case OperadorIgual:
			return math.Abs(v1Float-v2Float) < epsilon
		case OperadorDistinto:
			return math.Abs(v1Float-v2Float) >= epsilon
		case OperadorMayor:
			return v1Float > v2Float
		case OperadorMenor:
			return v1Float < v2Float
		default:
			return false
		}
	}

	// Tipos incompatibles (ej: número vs string)
	mr.logger.Printf("No se puede comparar %T con %T", valor1, valor2)
	return false
}

func (mr *MotorReglas) ejecutarAcciones(regla *Regla, timestamp time.Time) error {
	valores := make(map[string]interface{})

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

	// NUEVA VALIDACIÓN 1: Valor no puede ser nil
	if condicion.Valor == nil {
		return fmt.Errorf("valor de condición no puede ser nil")
	}

	// NUEVA VALIDACIÓN 2: Solo tipos soportados
	switch condicion.Valor.(type) {
	case bool, int64, float64, string:
		// OK
	default:
		return fmt.Errorf("tipo de valor no soportado: %T (use bool, int64, float64 o string)", condicion.Valor)
	}

	// NUEVA VALIDACIÓN 3: Operadores restringidos para bool/string
	switch condicion.Valor.(type) {
	case bool, string:
		if condicion.Operador != OperadorIgual && condicion.Operador != OperadorDistinto {
			return fmt.Errorf("tipo %T solo soporta operadores == y != (recibido: %s)",
				condicion.Valor, condicion.Operador)
		}
	}

	// NUEVA VALIDACIÓN 4: FiltroValor
	if condicion.FiltroValor != nil {
		// Solo permitir filtro con AgregacionCount (por ahora)
		if condicion.Agregacion != AgregacionCount {
			return fmt.Errorf("FiltroValor solo está soportado con agregación 'count' (recibido: %s)", condicion.Agregacion)
		}

		// Validar tipo de FiltroValor
		switch condicion.FiltroValor.(type) {
		case bool, int64, float64, string:
			// OK
		default:
			return fmt.Errorf("FiltroValor tipo no soportado: %T (use bool, int64, float64 o string)", condicion.FiltroValor)
		}
	}

	// NUEVA VALIDACIÓN 5: Agregaciones incompatibles con Text/Boolean
	if condicion.Agregacion != "" && condicion.Agregacion != AgregacionCount {
		// Verificar si alguna serie del grupo es Text o Boolean
		if err := mr.validarAgregacionCompatible(condicion); err != nil {
			return err
		}
	}

	return nil
}

// validarAgregacionCompatible verifica que la agregación sea compatible con los tipos de las series
func (mr *MotorReglas) validarAgregacionCompatible(condicion *Condicion) error {
	if mr.manager == nil {
		return nil // No podemos validar sin manager
	}

	// Lista de series a validar
	var seriesToValidate []string

	if condicion.Serie != "" {
		seriesToValidate = append(seriesToValidate, condicion.Serie)
	}
	if len(condicion.SeriesGrupo) > 0 {
		seriesToValidate = append(seriesToValidate, condicion.SeriesGrupo...)
	}

	// Validar cada serie (no-estricto: solo si existe)
	tiposEncontrados := make(map[tipos.TipoDatos]bool)

	for _, path := range seriesToValidate {
		serie, err := mr.manager.ObtenerSeries(path)
		if err != nil {
			// Serie no existe todavía - skip (validación no-estricta)
			continue
		}

		tiposEncontrados[serie.TipoDatos] = true

		// Text y Boolean NO permiten agregaciones (excepto count)
		if serie.TipoDatos == tipos.Text || serie.TipoDatos == tipos.Boolean {
			return fmt.Errorf("agregación '%s' no soportada para serie '%s' de tipo %s (solo 'count' es válido)",
				condicion.Agregacion, path, serie.TipoDatos)
		}
	}

	// VALIDACIÓN ADICIONAL: Grupos heterogéneos no permiten agregaciones
	if len(tiposEncontrados) > 1 {
		return fmt.Errorf("no se puede agregar series de tipos diferentes (encontrados: %v)",
			getTiposKeys(tiposEncontrados))
	}

	return nil
}

// Helper para extraer keys de map
func getTiposKeys(m map[tipos.TipoDatos]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k.String())
	}
	return keys
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
	return tipos.SerializarGob(regla)
}

func deserializarRegla(data []byte) (*Regla, error) {
	var regla Regla
	if err := tipos.DeserializarGob(data, &regla); err != nil {
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
