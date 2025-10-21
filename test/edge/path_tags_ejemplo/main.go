package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cbiale/sensorwave/edge"
)

func main() {
	fmt.Println("=== EJEMPLO DE USO: MODELO PATH + TAGS ===\n")

	manager, err := edge.Crear("ejemplo_path_tags.db")
	if err != nil {
		fmt.Println("Error al crear manager:", err)
		return
	}
	defer func() {
		manager.Cerrar()
		os.Remove("ejemplo_path_tags.db")
	}()

	fmt.Println("✓ Manager Edge inicializado\n")

	// ===== CREAR SERIES CON PATH Y TAGS =====
	fmt.Println("--- Creando series con Path y Tags ---")

	// Dispositivo 1: Sensor de temperatura en sala 1
	serie1 := edge.Serie{
		Path: "dispositivo_001/temperatura",
		Tags: map[string]string{
			"ubicacion": "sala1",
			"tipo":      "DHT22",
			"zona":      "produccion",
			"edificio":  "norte",
		},
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.LZ4,
		CompresionBytes:  edge.DeltaDelta,
		TamañoBloque:     100,
	}
	manager.CrearSerie(serie1)
	fmt.Printf("✓ Serie creada: %s\n", serie1.Path)
	fmt.Printf("  Tags: %v\n", serie1.Tags)

	// Dispositivo 1: Sensor de humedad
	serie2 := edge.Serie{
		Path: "dispositivo_001/humedad",
		Tags: map[string]string{
			"ubicacion": "sala1",
			"tipo":      "DHT22",
			"zona":      "produccion",
			"edificio":  "norte",
		},
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.LZ4,
		CompresionBytes:  edge.DeltaDelta,
		TamañoBloque:     100,
	}
	manager.CrearSerie(serie2)
	fmt.Printf("✓ Serie creada: %s\n", serie2.Path)

	// Dispositivo 2: Sensor de temperatura en sala 2
	serie3 := edge.Serie{
		Path: "dispositivo_002/temperatura",
		Tags: map[string]string{
			"ubicacion": "sala2",
			"tipo":      "DS18B20",
			"zona":      "almacen",
			"edificio":  "norte",
		},
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.ZSTD,
		CompresionBytes:  edge.DeltaDelta,
		TamañoBloque:     100,
	}
	manager.CrearSerie(serie3)
	fmt.Printf("✓ Serie creada: %s\n", serie3.Path)

	// Dispositivo 3: Sensor en edificio sur
	serie4 := edge.Serie{
		Path: "dispositivo_003/temperatura",
		Tags: map[string]string{
			"ubicacion": "oficina1",
			"tipo":      "DHT22",
			"zona":      "oficinas",
			"edificio":  "sur",
		},
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.Snappy,
		CompresionBytes:  edge.DeltaDelta,
		TamañoBloque:     100,
	}
	manager.CrearSerie(serie4)
	fmt.Printf("✓ Serie creada: %s\n\n", serie4.Path)

	// ===== INSERTAR DATOS =====
	fmt.Println("--- Insertando datos de ejemplo ---")
	now := time.Now()

	// Datos para dispositivo 1
	for i := 0; i < 10; i++ {
		timestamp := now.Add(time.Duration(i) * time.Second).UnixNano()
		manager.Insertar("dispositivo_001/temperatura", timestamp, 23.5+float64(i)*0.1)
		manager.Insertar("dispositivo_001/humedad", timestamp, 60.0+float64(i)*0.5)
	}
	fmt.Println("✓ Insertados 10 datos para dispositivo_001")

	// Datos para dispositivo 2
	for i := 0; i < 10; i++ {
		timestamp := now.Add(time.Duration(i) * time.Second).UnixNano()
		manager.Insertar("dispositivo_002/temperatura", timestamp, 25.0+float64(i)*0.2)
	}
	fmt.Println("✓ Insertados 10 datos para dispositivo_002")

	// Datos para dispositivo 3
	for i := 0; i < 10; i++ {
		timestamp := now.Add(time.Duration(i) * time.Second).UnixNano()
		manager.Insertar("dispositivo_003/temperatura", timestamp, 22.0+float64(i)*0.15)
	}
	fmt.Println("✓ Insertados 10 datos para dispositivo_003\n")

	// ===== CONSULTAS POR DISPOSITIVO =====
	fmt.Println("--- Consulta 1: Todas las series de dispositivo_001 ---")
	seriesDisp1, err := manager.ListarSeriesPorDispositivo("dispositivo_001")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Encontradas %d series:\n", len(seriesDisp1))
		for _, serie := range seriesDisp1 {
			fmt.Printf("  - %s (tags: %v)\n", serie.Path, serie.Tags)
		}
	}
	fmt.Println()

	// ===== CONSULTAS POR PATH PATTERN =====
	fmt.Println("--- Consulta 2: Todas las temperaturas (wildcard) ---")
	seriesTemp, err := manager.ListarSeriesPorPath("*/temperatura")
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Encontradas %d series de temperatura:\n", len(seriesTemp))
		for _, serie := range seriesTemp {
			fmt.Printf("  - %s (ubicación: %s, tipo: %s)\n",
				serie.Path, serie.Tags["ubicacion"], serie.Tags["tipo"])
		}
	}
	fmt.Println()

	// ===== CONSULTAS POR TAGS =====
	fmt.Println("--- Consulta 3: Series en zona 'produccion' ---")
	seriesProduccion, err := manager.ListarSeriesPorTags(map[string]string{
		"zona": "produccion",
	})
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Encontradas %d series en producción:\n", len(seriesProduccion))
		for _, serie := range seriesProduccion {
			fmt.Printf("  - %s (ubicación: %s)\n", serie.Path, serie.Tags["ubicacion"])
		}
	}
	fmt.Println()

	fmt.Println("--- Consulta 4: Series DHT22 en edificio norte ---")
	seriesDHT22Norte, err := manager.ListarSeriesPorTags(map[string]string{
		"tipo":     "DHT22",
		"edificio": "norte",
	})
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Encontradas %d series DHT22 en edificio norte:\n", len(seriesDHT22Norte))
		for _, serie := range seriesDHT22Norte {
			fmt.Printf("  - %s (zona: %s, ubicación: %s)\n",
				serie.Path, serie.Tags["zona"], serie.Tags["ubicacion"])
		}
	}
	fmt.Println()

	// ===== CONSULTAR DATOS CON RANGO TEMPORAL =====
	fmt.Println("--- Consulta 5: Datos de temperatura de dispositivo_001 ---")
	mediciones, err := manager.ConsultarRangoPorPath(
		"dispositivo_001/temperatura",
		now.Add(-1*time.Minute),
		now.Add(1*time.Minute),
	)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Encontradas %d mediciones:\n", len(mediciones))
		for i, m := range mediciones {
			if i < 5 { // Mostrar solo las primeras 5
				valor := m.Valor.(float64)
				fmt.Printf("  - Timestamp: %v, Valor: %.2f°C\n",
					time.Unix(0, m.Tiempo).Format("15:04:05"), valor)
			}
		}
		if len(mediciones) > 5 {
			fmt.Printf("  ... y %d mediciones más\n", len(mediciones)-5)
		}
	}
	fmt.Println()

	// ===== CREAR REGLA CON PATH Y TAGS =====
	fmt.Println("--- Ejemplo: Regla con PathPattern y TagsFilter ---")

	// Registrar ejecutor de ejemplo
	manager.RegistrarEjecutor("alerta_temperatura", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		fmt.Printf("🚨 ALERTA: %s - Valores: %v\n", accion.Destino, valores)
		return nil
	})

	regla := &edge.Regla{
		ID:     "temp_alta_produccion",
		Nombre: "Alerta Temperatura Alta en Producción",
		Condiciones: []edge.Condicion{
			{
				PathPattern: "*/temperatura", // Todas las temperaturas
				TagsFilter: map[string]string{
					"zona": "produccion", // Solo en zona de producción
				},
				Agregacion: edge.AgregacionPromedio, // Requerido para PathPattern/Tags
				Operador:   edge.OperadorMayor,
				Valor:      30.0,
				VentanaT:   30 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "alerta_temperatura",
				Destino: "Temperatura alta detectada en zona de producción",
			},
		},
		Logica: edge.LogicaAND,
	}

	err = manager.AgregarRegla(regla)
	if err != nil {
		fmt.Println("Error al agregar regla:", err)
	} else {
		fmt.Println("✓ Regla agregada exitosamente")
		fmt.Printf("  - ID: %s\n", regla.ID)
		fmt.Printf("  - Nombre: %s\n", regla.Nombre)
		fmt.Printf("  - PathPattern: %s\n", regla.Condiciones[0].PathPattern)
		fmt.Printf("  - TagsFilter: %v\n", regla.Condiciones[0].TagsFilter)
	}
	fmt.Println()

	// ===== RESUMEN FINAL =====
	fmt.Println("--- Resumen del Sistema ---")
	todasSeries, _ := manager.ListarSeries()
	fmt.Printf("Total de series creadas: %d\n", len(todasSeries))

	reglas := manager.ListarReglas()
	fmt.Printf("Total de reglas configuradas: %d\n", len(reglas))

	fmt.Println("\n🎉 Ejemplo completado exitosamente!")
	fmt.Println("\nVentajas del modelo Path + Tags:")
	fmt.Println("  ✓ Organización jerárquica clara (Path)")
	fmt.Println("  ✓ Filtrado flexible por metadatos (Tags)")
	fmt.Println("  ✓ Consultas eficientes por dispositivo, ubicación, tipo, etc.")
	fmt.Println("  ✓ Compatible con TSDBs modernas (IoTDB, Prometheus, InfluxDB)")
}
