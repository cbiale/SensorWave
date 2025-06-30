package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	edgesensorwave "edgesensorwave/pkg"
	"edgesensorwave/pkg/motor"
)

func main() {
	fmt.Println("=== EdgeSensorWave - Ejemplo Básico ===")
	fmt.Printf("Versión: %s (Motor: %s)\n\n", edgesensorwave.Version(), edgesensorwave.VersionMotor())

	// Base de datos de prueba
	dbPath := "./test_sensores.esw"
	os.RemoveAll(dbPath)

	// 1. Abrir base de datos con opciones por defecto
	fmt.Println("🔗 Abriendo base de datos...")
	db, err := edgesensorwave.Abrir(dbPath, edgesensorwave.OpcionesDefecto())
	if err != nil {
		log.Fatalf("Error abriendo base de datos: %v", err)
	}
	defer func() {
		fmt.Println("🔒 Cerrando base de datos...")
		db.Cerrar()
	}()

	// 2. Insertar datos individuales de sensores
	fmt.Println("📝 Insertando datos individuales...")
	ahora := time.Now()
	
	sensores := []struct {
		id    string
		valor float64
	}{
		{"temperatura.salon", 23.5},
		{"temperatura.cocina", 25.1},
		{"humedad.salon", 65.0},
		{"humedad.cocina", 70.2},
		{"presion.exterior", 1013.25},
	}

	for i, sensor := range sensores {
		timestamp := ahora.Add(time.Duration(i) * time.Minute)
		err := db.InsertarSensor(sensor.id, sensor.valor, timestamp)
		if err != nil {
			log.Printf("Error insertando %s: %v", sensor.id, err)
			continue
		}
		fmt.Printf("  ✓ %s: %.2f (%v)\n", sensor.id, sensor.valor, timestamp.Format("15:04:05"))
	}

	// 3. Insertar datos con calidad y metadatos
	fmt.Println("\n📊 Insertando datos con calidad y metadatos...")
	metadatos := map[string]string{
		"ubicacion": "planta_baja",
		"tipo":      "DHT22",
		"calibrado": "2024-01-15",
	}
	
	err = db.InsertarSensorConCalidad(
		"temperatura.calibrado", 
		24.3, 
		motor.CalidadBuena, 
		ahora.Add(10*time.Minute), 
		metadatos,
	)
	if err != nil {
		log.Printf("Error insertando sensor calibrado: %v", err)
	} else {
		fmt.Println("  ✓ temperatura.calibrado con metadatos")
	}

	// 4. Inserción en lotes para alto rendimiento
	fmt.Println("\n⚡ Insertando datos en lotes (simulando alta frecuencia)...")
	inicio := time.Now()
	
	lote := db.NuevoLote()
	numRegistros := 1000
	
	for i := 0; i < numRegistros; i++ {
		timestamp := ahora.Add(time.Duration(i) * time.Second)
		
		// Simular datos de múltiples sensores
		for _, prefijo := range []string{"cpu", "memoria", "disco", "red"} {
			sensorID := fmt.Sprintf("%s.servidor_%d", prefijo, i%10)
			valor := rand.Float64() * 100
			
			err := lote.Agregar(sensorID, valor, timestamp)
			if err != nil {
				log.Printf("Error agregando al lote: %v", err)
				break
			}
		}
	}
	
	// Confirmar el lote
	err = db.ConfirmarLote(lote)
	if err != nil {
		log.Printf("Error confirmando lote: %v", err)
	} else {
		duracion := time.Since(inicio)
		totalInserciones := numRegistros * 4 // 4 sensores por iteración
		velocidad := float64(totalInserciones) / duracion.Seconds()
		fmt.Printf("  ✓ %d registros insertados en %v (%.0f registros/seg)\n", 
			totalInserciones, duracion, velocidad)
	}

	// 5. Consultas básicas - buscar sensor específico
	fmt.Println("\n🔍 Consultas básicas:")
	valor, err := db.BuscarSensor("temperatura.salon", ahora)
	if err != nil {
		log.Printf("Error buscando sensor: %v", err)
	} else if valor != nil {
		fmt.Printf("  ✓ temperatura.salon en %v: %.2f (calidad: %s)\n", 
			ahora.Format("15:04:05"), valor.Valor, valor.Calidad)
	} else {
		fmt.Println("  ℹ️ temperatura.salon no encontrado en timestamp exacto")
	}

	// 6. Consultas por rango temporal
	fmt.Println("\n📈 Consultas por rango temporal:")
	hace1Hora := ahora.Add(-time.Hour)
	
	iter, err := db.ConsultarRango("temperatura.*", hace1Hora, ahora.Add(time.Hour))
	if err != nil {
		log.Printf("Error consultando rango: %v", err)
	} else {
		defer iter.Cerrar()
		
		fmt.Println("  Sensores de temperatura encontrados:")
		contador := 0
		for iter.Siguiente() && contador < 5 { // Limitar salida
			clave := iter.Clave()
			valor := iter.Valor()
			fmt.Printf("    • %s: %.2f (%s) [%v]\n", 
				clave.IDSensor, valor.Valor, valor.Calidad, 
				clave.Timestamp.Format("15:04:05"))
			contador++
		}
		if contador == 5 {
			fmt.Println("    ... (mostrando solo los primeros 5)")
		}
	}

	// 7. Consultas avanzadas - estadísticas
	fmt.Println("\n📊 Estadísticas de sensores:")
	stats, err := db.CalcularEstadisticas("cpu.*", hace1Hora, ahora.Add(time.Hour))
	if err != nil {
		log.Printf("Error calculando estadísticas: %v", err)
	} else {
		fmt.Printf("  CPU - Registros: %d, Min: %.2f, Max: %.2f, Promedio: %.2f\n",
			stats.NumRegistros, stats.ValorMinimo, stats.ValorMaximo, stats.ValorPromedio)
	}

	// 8. Listar todos los sensores únicos
	fmt.Println("\n📋 Listado de sensores:")
	sensoresUnicos, err := db.ListarSensores("*")
	if err != nil {
		log.Printf("Error listando sensores: %v", err)
	} else {
		fmt.Printf("  Total de sensores únicos: %d\n", len(sensoresUnicos))
		fmt.Println("  Ejemplos:")
		for i, sensor := range sensoresUnicos {
			if i >= 10 { // Mostrar solo los primeros 10
				fmt.Printf("    ... y %d más\n", len(sensoresUnicos)-10)
				break
			}
			fmt.Printf("    • %s\n", sensor)
		}
	}

	// 9. Agregación por intervalos
	fmt.Println("\n📈 Agregación por intervalos (últimos datos por minuto):")
	agregados, err := db.AgregarPorIntervalo("cpu.servidor_0", hace1Hora, ahora.Add(time.Hour), time.Minute)
	if err != nil {
		log.Printf("Error en agregación: %v", err)
	} else {
		fmt.Printf("  Intervalos encontrados: %d\n", len(agregados))
		if len(agregados) > 0 {
			fmt.Println("  Últimos 3 intervalos:")
			inicio := len(agregados) - 3
			if inicio < 0 {
				inicio = 0
			}
			for i := inicio; i < len(agregados); i++ {
				agg := agregados[i]
				fmt.Printf("    %v: Prom=%.2f, Min=%.2f, Max=%.2f (n=%d)\n",
					agg.Timestamp.Format("15:04"), agg.Promedio, agg.Minimo, agg.Maximo, agg.Conteo)
			}
		}
	}

	// 10. Buscar último valor de sensores
	fmt.Println("\n🔚 Últimos valores registrados:")
	for _, sensorNombre := range []string{"temperatura.salon", "cpu.servidor_0", "memoria.servidor_1"} {
		clave, valor, err := db.BuscarUltimo(sensorNombre)
		if err != nil {
			log.Printf("Error buscando último %s: %v", sensorNombre, err)
			continue
		}
		if clave != nil && valor != nil {
			fmt.Printf("  • %s: %.2f (%v)\n", 
				sensorNombre, valor.Valor, clave.Timestamp.Format("15:04:05"))
		} else {
			fmt.Printf("  • %s: sin datos\n", sensorNombre)
		}
	}

	// 11. Estadísticas generales de la base de datos
	fmt.Println("\n📊 Estadísticas generales de la base de datos:")
	statsDB, err := db.Estadisticas()
	if err != nil {
		log.Printf("Error obteniendo estadísticas: %v", err)
	} else {
		fmt.Printf("  • Sensores únicos: %d\n", statsDB.NumSensores)
		fmt.Printf("  • Registros totales: %d\n", statsDB.NumRegistros)
		fmt.Printf("  • Tamaño en disco: %s\n", edgesensorwave.FormatearBytes(statsDB.TamañoBytes))
		fmt.Printf("  • Última compactación: %v\n", statsDB.UltimaCompactacion.Format("2006-01-02 15:04:05"))
	}

	// 12. Operaciones de mantenimiento
	fmt.Println("\n🔧 Operaciones de mantenimiento:")
	
	// Sincronizar datos
	if err := db.Sincronizar(); err != nil {
		log.Printf("Error sincronizando: %v", err)
	} else {
		fmt.Println("  ✓ Datos sincronizados al disco")
	}
	
	// Compactar base de datos
	if err := db.Compactar(); err != nil {
		log.Printf("Error compactando: %v", err)
	} else {
		fmt.Println("  ✓ Base de datos compactada")
	}

	// 13. Demostrar diferentes configuraciones
	fmt.Println("\n⚙️ Diferentes configuraciones disponibles:")
	fmt.Println("  • OpcionesDefecto(): Balance entre rendimiento y memoria")
	fmt.Println("  • OpcionesRendimiento(): Máximo rendimiento (más memoria)")
	fmt.Println("  • OpcionesMemoriaMinima(): Mínimo uso de memoria")

	// Ejemplo de validaciones
	fmt.Println("\n✅ Validaciones:")
	
	// Validar ID de sensor
	idsTest := []string{"valido.sensor", "inv@lido", "", "muy.muy.muy.largo.sensor.con.muchos.niveles.de.jerarquia"}
	for _, id := range idsTest {
		if err := edgesensorwave.ValidarIDSensor(id); err != nil {
			fmt.Printf("  ❌ '%s': %v\n", id, err)
		} else {
			fmt.Printf("  ✅ '%s': válido\n", id)
		}
	}

	fmt.Println("\n✨ ¡Ejemplo completado exitosamente!")
	fmt.Println("   EdgeSensorWave está listo para usar en tu aplicación IoT edge.")
}