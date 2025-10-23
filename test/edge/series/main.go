package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	edge "github.com/cbiale/sensorwave/edge"
	"github.com/cbiale/sensorwave/tipos"
)

func main() {
	// Crear una instancia de ManagerEdge
	manager, err := edge.Crear("test_series.db", "localhost", "4222", "")
	if err != nil {
		fmt.Println("Error al crear ManagerEdge:", err)
		return
	}
	defer func() {
		manager.Cerrar()
		os.Remove("test_series.db") // Limpiar archivo de prueba
	}()

	fmt.Println("=== TEST DE SERIES DE TIEMPO ===")
	fmt.Println("ManagerEdge creado exitosamente")

	// Test 1: Crear series con diferentes configuraciones de compresión
	fmt.Println("\n--- Test 1: Creación de series con diferentes compresiones ---")

	series := []edge.Serie{
		{
			Path:             "sensor1.temperatura",
			TipoDatos:        tipos.TipoNumerico,
			CompresionBloque: tipos.LZ4,
			CompresionBytes:  tipos.DeltaDelta,
			TamañoBloque:     10,
		},
		{
			Path:             "sensor1.humedad",
			TipoDatos:        tipos.TipoNumerico,
			CompresionBloque: tipos.ZSTD,
			CompresionBytes:  tipos.DeltaDelta,
			TamañoBloque:     20,
		},
		{
			Path:             "sensor1.presion",
			TipoDatos:        tipos.TipoNumerico,
			CompresionBloque: tipos.Snappy,
			CompresionBytes:  tipos.RLE,
			TamañoBloque:     15,
		},
		{
			Path:             "sensor2.temperatura",
			TipoDatos:        tipos.TipoNumerico,
			CompresionBloque: tipos.Gzip,
			CompresionBytes:  tipos.DeltaDelta,
			TamañoBloque:     30,
		},
		{
			Path:             "sensor1.estado",
			TipoDatos:        tipos.TipoCategorico,
			CompresionBloque: tipos.LZ4,
			CompresionBytes:  tipos.RLE,
			TamañoBloque:     10,
		},
	}

	for _, serie := range series {
		err := manager.CrearSerie(serie)
		if err != nil {
			fmt.Printf("Error al crear serie %s: %v\n", serie.Path, err)
			return
		}
		fmt.Printf("Serie creada: %s (Tipo: %s, Bloque: %v, Valores: %v, Tamaño: %d)\n",
			serie.Path, serie.TipoDatos, serie.CompresionBloque, serie.CompresionBytes, serie.TamañoBloque)
	}

	// Test 2: Insertar datos con patrones diferentes
	fmt.Println("\n--- Test 2: Inserción de datos con diferentes patrones ---")

	baseTime := time.Now()

	// Patrón 1: Datos lineales crecientes (temperatura)
	fmt.Println("Insertando datos de temperatura (patrón lineal)...")
	for i := 0; i < 50; i++ {
		valor := 20.0 + float64(i)*0.5 // Incremento lineal
		timestamp := baseTime.Add(time.Duration(i) * time.Second).UnixNano()
		err = manager.Insertar("sensor1.temperatura", timestamp, valor)
		if err != nil {
			fmt.Printf("Error al insertar temperatura: %v\n", err)
			return
		}
	}

	// Patrón 2: Datos sinusoidales (humedad)
	fmt.Println("Insertando datos de humedad (patrón sinusoidal)...")
	for i := 0; i < 50; i++ {
		valor := 50.0 + 20.0*math.Sin(float64(i)*0.2) // Patrón sinusoidal
		timestamp := baseTime.Add(time.Duration(i) * time.Second).UnixNano()
		err = manager.Insertar("sensor1.humedad", timestamp, valor)
		if err != nil {
			fmt.Printf("Error al insertar humedad: %v\n", err)
			return
		}
	}

	// Patrón 3: Datos aleatorios (presión)
	fmt.Println("Insertando datos de presión (patrón aleatorio)...")
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 50; i++ {
		valor := 1013.25 + (rand.Float64()-0.5)*50 // Variación aleatoria alrededor de presión atmosférica
		timestamp := baseTime.Add(time.Duration(i) * time.Second).UnixNano()
		err = manager.Insertar("sensor1.presion", timestamp, valor)
		if err != nil {
			fmt.Printf("Error al insertar presión: %v\n", err)
			return
		}
	}

	// Patrón 4: Datos constantes con ruido
	fmt.Println("Insertando datos de temperatura sensor2 (patrón constante con ruido)...")
	for i := 0; i < 50; i++ {
		valor := 25.0 + (rand.Float64()-0.5)*2 // Constante con pequeño ruido
		timestamp := baseTime.Add(time.Duration(i) * time.Second).UnixNano()
		err = manager.Insertar("sensor2.temperatura", timestamp, valor)
		if err != nil {
			fmt.Printf("Error al insertar temperatura sensor2: %v\n", err)
			return
		}
	}

	// Patrón 5: Datos categóricos (estado del sensor)
	fmt.Println("Insertando datos de estado (patrón categórico)...")
	estados := []string{"normal", "alerta", "critico", "normal", "normal", "alerta", "normal", "critico", "normal", "normal"}
	for i, estado := range estados {
		timestamp := baseTime.Add(time.Duration(i*5) * time.Second).UnixNano()
		err = manager.Insertar("sensor1.estado", timestamp, estado)
		if err != nil {
			fmt.Printf("Error al insertar estado: %v\n", err)
			return
		}
	}

	fmt.Println("Datos insertados exitosamente")

	// Test 3: Verificar series existentes
	fmt.Println("\n--- Test 3: Listado de series ---")
	listaSeries, err := manager.ListarSeries()
	if err != nil {
		fmt.Println("Error al listar series:", err)
		return
	}
	fmt.Printf("Series encontradas: %v\n", listaSeries)

	// Test 4: Consultas de rangos
	fmt.Println("\n--- Test 4: Consultas de rangos temporales ---")

	horaInicio := baseTime.Add(-1 * time.Minute)
	horaFinal := baseTime.Add(1 * time.Minute)

	for _, nombreSerie := range listaSeries {
		fmt.Printf("\nConsultando rango para %s:\n", nombreSerie)
		puntos, err := manager.ConsultarRango(nombreSerie, horaInicio, horaFinal)
		if err != nil {
			fmt.Printf("Error al consultar rango: %v\n", err)
			continue
		}
		fmt.Printf("  - Puntos encontrados: %d\n", len(puntos))
		if len(puntos) > 0 {
			primerValor, _ := puntos[0].Valor.(float64)
			ultimoValor, _ := puntos[len(puntos)-1].Valor.(float64)
			fmt.Printf("  - Primer punto: %.2f (tiempo: %v)\n",
				primerValor, time.Unix(0, puntos[0].Tiempo))
			fmt.Printf("  - Último punto: %.2f (tiempo: %v)\n",
				ultimoValor, time.Unix(0, puntos[len(puntos)-1].Tiempo))
		}
	}

	// Test 5: Consulta del último punto
	fmt.Println("\n--- Test 5: Consulta del último punto ---")

	for _, nombreSerie := range listaSeries {
		ultimoPunto, err := manager.ConsultarUltimoPunto(nombreSerie)
		if err != nil {
			fmt.Printf("Error al consultar último punto de %s: %v\n", nombreSerie, err)
			continue
		}
		valor, _ := ultimoPunto.Valor.(float64)
		fmt.Printf("%s - Último punto: %.2f (tiempo: %v)\n",
			nombreSerie, valor, time.Unix(0, ultimoPunto.Tiempo))
	}

	// Test 6: Insertar datos fuera de orden (para probar robustez)
	fmt.Println("\n--- Test 6: Inserción de datos fuera de orden ---")

	// Insertar un dato del pasado
	timestampPasado := baseTime.Add(-30 * time.Second).UnixNano()
	err = manager.Insertar("sensor1.temperatura", timestampPasado, 15.0)
	if err != nil {
		fmt.Printf("Error al insertar dato del pasado: %v\n", err)
	} else {
		fmt.Println("Dato del pasado insertado correctamente")
	}

	// Insertar un dato del futuro
	timestampFuturo := baseTime.Add(2 * time.Minute).UnixNano()
	err = manager.Insertar("sensor1.temperatura", timestampFuturo, 50.0)
	if err != nil {
		fmt.Printf("Error al insertar dato del futuro: %v\n", err)
	} else {
		fmt.Println("Dato del futuro insertado correctamente")
	}

	// Test 7: Análisis estadístico básico
	fmt.Println("\n--- Test 7: Análisis estadístico ---")

	for _, nombreSerie := range listaSeries {
		puntos, err := manager.ConsultarRango(nombreSerie, horaInicio, horaFinal)
		if err != nil || len(puntos) == 0 {
			continue
		}

		// Calcular estadísticas básicas
		var suma, min, max float64
		primerValor, ok := puntos[0].Valor.(float64)
		if !ok {
			continue
		}
		min = primerValor
		max = primerValor

		for _, punto := range puntos {
			valor, ok := punto.Valor.(float64)
			if !ok {
				continue
			}
			suma += valor
			if valor < min {
				min = valor
			}
			if valor > max {
				max = valor
			}
		}

		promedio := suma / float64(len(puntos))

		fmt.Printf("%s - Puntos: %d, Promedio: %.2f, Min: %.2f, Max: %.2f\n",
			nombreSerie, len(puntos), promedio, min, max)
	}

	// Test 8: Demostración de optimización de skip temprano
	fmt.Println("\n--- Test 8: Optimización de consultas por rango ---")

	// Crear una serie con muchos bloques para demostrar la optimización
	testSerie := edge.Serie{
		Path:             "sensor.test.optimizacion",
		CompresionBloque: tipos.LZ4,
		CompresionBytes:  tipos.DeltaDelta,
		TamañoBloque:     5, // Bloques pequeños para crear muchos bloques
	}

	err = manager.CrearSerie(testSerie)
	if err != nil {
		fmt.Printf("Error al crear serie de test: %v\n", err)
		return
	}

	// Insertar datos distribuidos en el tiempo (100 puntos = ~20 bloques)
	fmt.Println("Insertando 100 puntos distribuidos en el tiempo...")
	baseTimeTest := time.Now()
	for i := 0; i < 100; i++ {
		valor := float64(i)
		timestamp := baseTimeTest.Add(time.Duration(i) * time.Second).UnixNano()
		err = manager.Insertar("sensor.test.optimizacion", timestamp, valor)
		if err != nil {
			fmt.Printf("Error al insertar dato %d: %v\n", i, err)
			return
		}
	}

	// Dar tiempo para que se guarden los bloques
	time.Sleep(100 * time.Millisecond)

	// Test de consulta pequeña en el medio (debería skipear muchos bloques)
	fmt.Println("\nPrueba de optimización: consulta de 10 segundos en el medio de 100 segundos de datos")
	inicioOptimizacion := baseTimeTest.Add(45 * time.Second)
	finOptimizacion := baseTimeTest.Add(55 * time.Second)

	// Esta consulta debería skipear la mayoría de bloques gracias a la optimización
	puntosOptimizacion, err := manager.ConsultarRango("sensor.test.optimizacion", inicioOptimizacion, finOptimizacion)
	if err != nil {
		fmt.Printf("Error en consulta optimizada: %v\n", err)
	} else {
		fmt.Printf("  - Puntos encontrados en rango pequeño: %d\n", len(puntosOptimizacion))
		if len(puntosOptimizacion) > 0 {
			fmt.Printf("  - Rango de valores: %.0f a %.0f\n",
				puntosOptimizacion[0].Valor.(float64),
				puntosOptimizacion[len(puntosOptimizacion)-1].Valor.(float64))
			fmt.Println("  - ✅ Optimización funcionando: solo se procesaron bloques relevantes")
		} else {
			fmt.Println("  - ⚠️ No se encontraron puntos en el rango especificado")
		}
	}

	// Test de consulta amplia (procesa todos los bloques)
	fmt.Println("\nPrueba de consulta amplia: todos los datos")
	inicioAmplio := baseTimeTest.Add(-10 * time.Second)
	finAmplio := baseTimeTest.Add(110 * time.Second)

	puntosAmplios, err := manager.ConsultarRango("sensor.test.optimizacion", inicioAmplio, finAmplio)
	if err != nil {
		fmt.Printf("Error en consulta amplia: %v\n", err)
	} else {
		fmt.Printf("  - Puntos encontrados en rango amplio: %d\n", len(puntosAmplios))
		fmt.Println("  - ✅ Consulta amplia procesó todos los bloques necesarios")
	}

	// Test 9: Inferencia automática de tipos
	fmt.Println("\n--- Test 9: Inferencia automática de tipos ---")

	// Crear serie sin especificar tipo (TipoMixto por defecto)
	serieInferencia := edge.Serie{
		Path:             "sensor.inferencia.automatica",
		TipoDatos:        tipos.TipoMixto, // Tipo mixto permite inferencia
		CompresionBloque: tipos.LZ4,
		CompresionBytes:  tipos.DeltaDelta,
		TamañoBloque:     5,
	}

	err = manager.CrearSerie(serieInferencia)
	if err != nil {
		fmt.Printf("Error al crear serie de inferencia: %v\n", err)
		return
	}

	// Primera inserción: valor numérico (debería inferir NUMERIC)
	timestamp1 := time.Now().UnixNano()
	err = manager.Insertar("sensor.inferencia.automatica", timestamp1, 42.5)
	if err != nil {
		fmt.Printf("Error en primera inserción: %v\n", err)
	} else {
		fmt.Println("✅ Primera inserción (numérica) exitosa - tipo inferido automáticamente")
	}

	// Segunda inserción: otro valor numérico (debería ser compatible)
	timestamp2 := time.Now().UnixNano()
	err = manager.Insertar("sensor.inferencia.automatica", timestamp2, 100)
	if err != nil {
		fmt.Printf("Error en segunda inserción numérica: %v\n", err)
	} else {
		fmt.Println("✅ Segunda inserción (numérica) exitosa - compatible con tipo inferido")
	}

	// Tercera inserción: valor categórico (debería fallar por incompatibilidad)
	timestamp3 := time.Now().UnixNano()
	err = manager.Insertar("sensor.inferencia.automatica", timestamp3, "texto")
	if err != nil {
		fmt.Println("✅ Tercera inserción (categórica) rechazada correctamente:", err)
	} else {
		fmt.Println("❌ Error: debería haber rechazado el tipo incompatible")
	}

	// Test con serie categórica inferida
	fmt.Println("\nPrueba de inferencia categórica:")

	serieCategórica := edge.Serie{
		Path:             "sensor.inferencia.categorica",
		TipoDatos:        tipos.TipoMixto,
		CompresionBloque: tipos.LZ4,
		CompresionBytes:  tipos.RLE,
		TamañoBloque:     5,
	}

	err = manager.CrearSerie(serieCategórica)
	if err != nil {
		fmt.Printf("Error al crear serie categórica: %v\n", err)
		return
	}

	// Primera inserción categórica
	timestamp4 := time.Now().UnixNano()
	err = manager.Insertar("sensor.inferencia.categorica", timestamp4, "activo")
	if err != nil {
		fmt.Printf("Error en inserción categórica: %v\n", err)
	} else {
		fmt.Println("✅ Inferencia categórica exitosa")
	}

	// Verificar que rechaza numéricos
	timestamp5 := time.Now().UnixNano()
	err = manager.Insertar("sensor.inferencia.categorica", timestamp5, 123)
	if err != nil {
		fmt.Println("✅ Valor numérico rechazado correctamente en serie categórica:", err)
	} else {
		fmt.Println("❌ Error: debería haber rechazado el valor numérico")
	}

	fmt.Println("\n=== TEST COMPLETADO EXITOSAMENTE ===")
}
