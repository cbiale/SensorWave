package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	edge "github.com/cbiale/sensorwave/edge"
)

type EstadoSistema struct {
	BombaActivada     bool
	VentiladorActivo  bool
	SistemaEmergencia bool
	UltimoRiego       time.Time
	ContadorRiegos    int
	ContadorAlertas   int
	TemperaturaMax    float64
	HumedadMin        float64
}

func main() {
	fmt.Println("=== SISTEMA INTEGRADO DE AGRICULTURA INTELIGENTE ===")

	// Crear instancia de ManagerEdge
	manager, err := edge.Crear("agricultura_inteligente.db", "localhost", "4222")
	if err != nil {
		fmt.Println("Error al crear ManagerEdge:", err)
		return
	}
	defer func() {
		manager.Cerrar()
		os.Remove("agricultura_inteligente.db") // Limpiar archivo de prueba
	}()

	fmt.Println("✓ Base de datos y motor de reglas inicializados")

	// Estado del sistema
	estado := &EstadoSistema{
		UltimoRiego:    time.Now().Add(-2 * time.Hour),
		TemperaturaMax: -999,
		HumedadMin:     999,
	}

	// Registrar ejecutores personalizados
	manager.RegistrarEjecutor("activar_bomba", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		if !estado.BombaActivada && time.Since(estado.UltimoRiego) > 30*time.Minute {
			estado.BombaActivada = true
			estado.UltimoRiego = time.Now()
			estado.ContadorRiegos++
			fmt.Printf("💧 BOMBA ACTIVADA [Riego #%d] - Regla: %s\n",
				estado.ContadorRiegos, regla.Nombre)
			fmt.Printf("   └─ Zona: %s, Motivo: %s\n",
				accion.Destino, accion.Params["motivo"])
		}
		return nil
	})

	manager.RegistrarEjecutor("desactivar_bomba", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		if estado.BombaActivada {
			estado.BombaActivada = false
			fmt.Printf("💧 BOMBA DESACTIVADA - Regla: %s\n", regla.Nombre)
			fmt.Printf("   └─ Motivo: %s\n", accion.Params["motivo"])
		}
		return nil
	})

	manager.RegistrarEjecutor("activar_ventilacion", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		if !estado.VentiladorActivo {
			estado.VentiladorActivo = true
			fmt.Printf("🌪️  VENTILACIÓN ACTIVADA - Temperatura: %.1f°C\n",
				valores["temperatura_promedio"])
		}
		return nil
	})

	manager.RegistrarEjecutor("desactivar_ventilacion", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		if estado.VentiladorActivo {
			estado.VentiladorActivo = false
			fmt.Printf("🌪️  VENTILACIÓN DESACTIVADA - Temperatura: %.1f°C\n",
				valores["temperatura_promedio"])
		}
		return nil
	})

	manager.RegistrarEjecutor("alerta_emergencia", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		if !estado.SistemaEmergencia {
			estado.SistemaEmergencia = true
			estado.ContadorAlertas++
			fmt.Printf("🚨 ALERTA DE EMERGENCIA [#%d]: %s\n",
				estado.ContadorAlertas, accion.Destino)
			for serie, valor := range valores {
				fmt.Printf("   └─ %s: %.1f\n", serie, valor)
			}
		}
		return nil
	})

	manager.RegistrarEjecutor("log_optimizacion", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		fmt.Printf("📊 OPTIMIZACIÓN: %s\n", accion.Destino)
		return nil
	})

	// Crear series para diferentes tipos de sensores
	fmt.Println("\n--- Configurando sensores del sistema ---")

	// Series de temperatura (8 dispositivos distribuidos)
	fmt.Println("Creando series de temperatura...")
	for i := 1; i <= 8; i++ {
		nombreTemp := fmt.Sprintf("invernadero.zona%d.temperatura", i)
		err = manager.CrearSerie(edge.Serie{
			NombreSerie:      nombreTemp,
			TipoDatos:        edge.TipoNumerico,
			CompresionBloque: edge.LZ4,
			CompresionBytes:  edge.DeltaDelta,
			TamañoBloque:     20,
		})
		if err != nil {
			fmt.Printf("Error al crear serie de temperatura %s: %v\n", nombreTemp, err)
			return
		}
		fmt.Printf("✓ %s\n", nombreTemp)
	}

	// Series de humedad del suelo (8 dispositivos)
	fmt.Println("Creando series de humedad del suelo...")
	for i := 1; i <= 8; i++ {
		nombreHumedad := fmt.Sprintf("invernadero.zona%d.humedad_suelo", i)
		err = manager.CrearSerie(edge.Serie{
			NombreSerie:      nombreHumedad,
			TipoDatos:        edge.TipoNumerico,
			CompresionBloque: edge.ZSTD,
			CompresionBytes:  edge.DeltaDelta,
			TamañoBloque:     20,
		})
		if err != nil {
			fmt.Printf("Error al crear serie de humedad %s: %v\n", nombreHumedad, err)
			return
		}
		fmt.Printf("✓ %s\n", nombreHumedad)
	}

	// Series adicionales para control ambiental
	seriesAdicionales := []string{
		"invernadero.humedad_ambiente",
		"invernadero.luz_solar",
		"invernadero.co2",
		"invernadero.ph_suelo",
		"invernadero.conductividad",
	}

	fmt.Println("Creando series de control ambiental...")
	for _, nombre := range seriesAdicionales {
		err = manager.CrearSerie(edge.Serie{
			NombreSerie:      nombre,
			TipoDatos:        edge.TipoNumerico,
			CompresionBloque: edge.Snappy,
			CompresionBytes:  edge.RLE,
			TamañoBloque:     15,
		})
		if err != nil {
			fmt.Printf("Error al crear serie %s: %v\n", nombre, err)
			return
		}
		fmt.Printf("✓ %s\n", nombre)
	}

	// Configurar reglas del sistema inteligente
	fmt.Println("\n--- Configurando reglas de automatización ---")

	// Regla 1: Activar riego por temperatura alta y humedad baja
	reglaActivarRiego := &edge.Regla{
		ID:     "activar_riego_inteligente",
		Nombre: "Activación Inteligente de Riego",
		Condiciones: []edge.Condicion{
			{
				SeriesGrupo: []string{
					"invernadero.zona1.temperatura", "invernadero.zona2.temperatura",
					"invernadero.zona3.temperatura", "invernadero.zona4.temperatura",
					"invernadero.zona5.temperatura", "invernadero.zona6.temperatura",
					"invernadero.zona7.temperatura", "invernadero.zona8.temperatura",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMayor,
				Valor:      25.0,
				VentanaT:   30 * time.Second,
			},
			{
				SeriesGrupo: []string{
					"invernadero.zona1.humedad_suelo", "invernadero.zona2.humedad_suelo",
					"invernadero.zona3.humedad_suelo", "invernadero.zona4.humedad_suelo",
					"invernadero.zona5.humedad_suelo", "invernadero.zona6.humedad_suelo",
					"invernadero.zona7.humedad_suelo", "invernadero.zona8.humedad_suelo",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMenor,
				Valor:      30.0,
				VentanaT:   30 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "activar_bomba",
				Destino: "sistema_riego_principal",
				Params:  map[string]string{"motivo": "condiciones_criticas", "duracion": "600s"},
			},
		},
		Logica: edge.LogicaAND,
	}

	// Regla 2: Desactivar riego por humedad suficiente
	reglaDesactivarRiego := &edge.Regla{
		ID:     "desactivar_riego_inteligente",
		Nombre: "Desactivación Automática de Riego",
		Condiciones: []edge.Condicion{
			{
				SeriesGrupo: []string{
					"invernadero.zona1.humedad_suelo", "invernadero.zona2.humedad_suelo",
					"invernadero.zona3.humedad_suelo", "invernadero.zona4.humedad_suelo",
					"invernadero.zona5.humedad_suelo", "invernadero.zona6.humedad_suelo",
					"invernadero.zona7.humedad_suelo", "invernadero.zona8.humedad_suelo",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMayorIgual,
				Valor:      65.0,
				VentanaT:   30 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "desactivar_bomba",
				Destino: "sistema_riego_principal",
				Params:  map[string]string{"motivo": "humedad_optima"},
			},
		},
		Logica: edge.LogicaAND,
	}

	// Regla 3: Control de ventilación por temperatura
	reglaVentilacion := &edge.Regla{
		ID:     "control_ventilacion",
		Nombre: "Control Automático de Ventilación",
		Condiciones: []edge.Condicion{
			{
				SeriesGrupo: []string{
					"invernadero.zona1.temperatura", "invernadero.zona2.temperatura",
					"invernadero.zona3.temperatura", "invernadero.zona4.temperatura",
					"invernadero.zona5.temperatura", "invernadero.zona6.temperatura",
					"invernadero.zona7.temperatura", "invernadero.zona8.temperatura",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMayor,
				Valor:      28.0,
				VentanaT:   20 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "activar_ventilacion",
				Destino: "sistema_ventilacion",
				Params:  map[string]string{"velocidad": "automatica"},
			},
		},
		Logica: edge.LogicaAND,
	}

	// Regla 4: Desactivar ventilación
	reglaDesactivarVentilacion := &edge.Regla{
		ID:     "desactivar_ventilacion",
		Nombre: "Desactivar Ventilación",
		Condiciones: []edge.Condicion{
			{
				SeriesGrupo: []string{
					"invernadero.zona1.temperatura", "invernadero.zona2.temperatura",
					"invernadero.zona3.temperatura", "invernadero.zona4.temperatura",
					"invernadero.zona5.temperatura", "invernadero.zona6.temperatura",
					"invernadero.zona7.temperatura", "invernadero.zona8.temperatura",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMenorIgual,
				Valor:      24.0,
				VentanaT:   20 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "desactivar_ventilacion",
				Destino: "sistema_ventilacion",
			},
		},
		Logica: edge.LogicaAND,
	}

	// Regla 5: Alerta de emergencia por condiciones extremas
	reglaEmergencia := &edge.Regla{
		ID:     "alerta_emergencia_temperatura",
		Nombre: "Alerta de Emergencia por Temperatura",
		Condiciones: []edge.Condicion{
			{
				SeriesGrupo: []string{
					"invernadero.zona1.temperatura", "invernadero.zona2.temperatura",
					"invernadero.zona3.temperatura", "invernadero.zona4.temperatura",
				},
				Agregacion: edge.AgregacionMaximo,
				Operador:   edge.OperadorMayor,
				Valor:      40.0,
				VentanaT:   15 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "alerta_emergencia",
				Destino: "TEMPERATURA CRÍTICA - Intervención manual requerida",
				Params:  map[string]string{"nivel": "critico", "tipo": "temperatura"},
			},
		},
		Logica: edge.LogicaAND,
	}

	// Regla 6: Optimización por condiciones ideales
	reglaOptimizacion := &edge.Regla{
		ID:     "optimizacion_condiciones",
		Nombre: "Optimización de Condiciones Ideales",
		Condiciones: []edge.Condicion{
			{
				SeriesGrupo: []string{
					"invernadero.zona1.temperatura", "invernadero.zona2.temperatura",
					"invernadero.zona3.temperatura", "invernadero.zona4.temperatura",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMayorIgual,
				Valor:      22.0,
				VentanaT:   60 * time.Second,
			},
			{
				SeriesGrupo: []string{
					"invernadero.zona1.temperatura", "invernadero.zona2.temperatura",
					"invernadero.zona3.temperatura", "invernadero.zona4.temperatura",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMenorIgual,
				Valor:      26.0,
				VentanaT:   60 * time.Second,
			},
			{
				SeriesGrupo: []string{
					"invernadero.zona1.humedad_suelo", "invernadero.zona2.humedad_suelo",
					"invernadero.zona3.humedad_suelo", "invernadero.zona4.humedad_suelo",
				},
				Agregacion: edge.AgregacionPromedio,
				Operador:   edge.OperadorMayorIgual,
				Valor:      45.0,
				VentanaT:   60 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "log_optimizacion",
				Destino: "Condiciones óptimas de cultivo alcanzadas",
				Params:  map[string]string{"estado": "optimo"},
			},
		},
		Logica: edge.LogicaAND,
	}

	// Agregar todas las reglas al motor
	reglas := []*edge.Regla{
		reglaActivarRiego,
		reglaDesactivarRiego,
		reglaVentilacion,
		reglaDesactivarVentilacion,
		reglaEmergencia,
		reglaOptimizacion,
	}

	fmt.Println("\nAgregando reglas al motor:")
	for _, regla := range reglas {
		err = manager.AgregarRegla(regla)
		if err != nil {
			fmt.Printf("Error al agregar regla %s: %v\n", regla.Nombre, err)
			return
		}
		fmt.Printf("✓ %s\n", regla.Nombre)
	}

	fmt.Println("\n✓ Sistema de automatización configurado correctamente")

	// Inicializar simulación avanzada
	fmt.Println("\n=== INICIANDO SIMULACIÓN AVANZADA ===")
	fmt.Println("Duración: 5 minutos simulados (tiempo acelerado)")

	// Inicializar valores de sensores para 8 zonas
	temperaturas := make([]float64, 8)
	humedadessuelo := make([]float64, 8)

	// Valores iniciales realistas
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 8; i++ {
		temperaturas[i] = 20.0 + rand.Float64()*3    // 20-23°C inicial
		humedadessuelo[i] = 35.0 + rand.Float64()*15 // 35-50% humedad inicial
	}

	// Variables para sensores ambientales
	humedadAmbiente := 60.0
	luzSolar := 40.0
	co2 := 400.0
	phSuelo := 6.8
	conductividad := 1.2

	fmt.Printf("Estado inicial del sistema:\n")
	fmt.Printf("  - Bomba: %v | Ventilador: %v | Emergencia: %v\n",
		estado.BombaActivada, estado.VentiladorActivo, estado.SistemaEmergencia)

	// Simular 5 minutos (300 segundos) con diferentes fases
	duracionTotal := 300
	for segundo := 0; segundo < duracionTotal; segundo++ {
		ahora := time.Now()

		// Fase 1: Calentamiento gradual (primeros 60 segundos)
		if segundo < 60 {
			// Simular aumento de temperatura por radiación solar
			factorCalentamiento := float64(segundo) / 60.0
			luzSolar = 40.0 + factorCalentamiento*50.0 // 40-90%

			for i := 0; i < 8; i++ {
				temperaturas[i] += rand.Float64()*0.15 + 0.05 // Incremento gradual
				humedadessuelo[i] -= rand.Float64() * 0.1     // Evaporación lenta
			}
		}

		// Fase 2: Calor intenso (segundos 60-120)
		if segundo >= 60 && segundo < 120 {
			luzSolar = 90.0 + rand.Float64()*10.0

			for i := 0; i < 8; i++ {
				temperaturas[i] += rand.Float64()*0.3 + 0.1 // Incremento más rápido
				if !estado.BombaActivada {
					humedadessuelo[i] -= rand.Float64()*0.2 + 0.1 // Evaporación acelerada
				}
			}
		}

		// Fase 3: Estabilización con riego (segundos 120-240)
		if segundo >= 120 && segundo < 240 {
			for i := 0; i < 8; i++ {
				if estado.BombaActivada {
					humedadessuelo[i] += rand.Float64()*1.0 + 0.5 // Incremento por riego
					temperaturas[i] -= rand.Float64() * 0.1       // Enfriamiento por evaporación
				} else {
					humedadessuelo[i] -= rand.Float64() * 0.15
				}

				if estado.VentiladorActivo {
					temperaturas[i] -= rand.Float64()*0.2 + 0.1 // Enfriamiento por ventilación
				}
			}
		}

		// Fase 4: Condiciones extremas para probar emergencias (segundos 240-300)
		if segundo >= 240 {
			if segundo < 270 { // 30 segundos de calor extremo
				for i := 0; i < 4; i++ { // Solo algunas zonas
					temperaturas[i] += rand.Float64()*0.5 + 0.3
				}
			}
		}

		// Actualizar sensores ambientales
		humedadAmbiente += (rand.Float64() - 0.5) * 2.0
		co2 += (rand.Float64() - 0.5) * 10.0
		phSuelo += (rand.Float64() - 0.5) * 0.02
		conductividad += (rand.Float64() - 0.5) * 0.05

		// Limitar valores a rangos realistas
		for i := 0; i < 8; i++ {
			if temperaturas[i] < 15.0 {
				temperaturas[i] = 15.0
			}
			if temperaturas[i] > 45.0 {
				temperaturas[i] = 45.0
			}
			if humedadessuelo[i] < 5.0 {
				humedadessuelo[i] = 5.0
			}
			if humedadessuelo[i] > 85.0 {
				humedadessuelo[i] = 85.0
			}
		}

		// Actualizar estadísticas del sistema
		tempMax := temperaturas[0]
		humMin := humedadessuelo[0]
		for i := 1; i < 8; i++ {
			if temperaturas[i] > tempMax {
				tempMax = temperaturas[i]
			}
			if humedadessuelo[i] < humMin {
				humMin = humedadessuelo[i]
			}
		}
		if tempMax > estado.TemperaturaMax {
			estado.TemperaturaMax = tempMax
		}
		if humMin < estado.HumedadMin {
			estado.HumedadMin = humMin
		}

		// Insertar datos en las series y procesar reglas
		timestamp := ahora.UnixNano()
		for i := 0; i < 8; i++ {
			nombreTemp := fmt.Sprintf("invernadero.zona%d.temperatura", i+1)
			nombreHumedad := fmt.Sprintf("invernadero.zona%d.humedad_suelo", i+1)

			// Insertar en base de datos
			manager.Insertar(nombreTemp, timestamp, temperaturas[i])
			manager.Insertar(nombreHumedad, timestamp, humedadessuelo[i])

			// Procesar en motor de reglas
			manager.ProcesarDatoRegla(nombreTemp, temperaturas[i], ahora)
			manager.ProcesarDatoRegla(nombreHumedad, humedadessuelo[i], ahora)
		}

		// Insertar datos de sensores ambientales
		manager.Insertar("invernadero.humedad_ambiente", timestamp, humedadAmbiente)
		manager.Insertar("invernadero.luz_solar", timestamp, luzSolar)
		manager.Insertar("invernadero.co2", timestamp, co2)
		manager.Insertar("invernadero.ph_suelo", timestamp, phSuelo)
		manager.Insertar("invernadero.conductividad", timestamp, conductividad)

		// Mostrar estado cada 30 segundos
		if segundo%30 == 0 {
			tempPromedio := 0.0
			humedadPromedio := 0.0
			for i := 0; i < 8; i++ {
				tempPromedio += temperaturas[i]
				humedadPromedio += humedadessuelo[i]
			}
			tempPromedio /= 8
			humedadPromedio /= 8

			estadoBomba := "❌"
			if estado.BombaActivada {
				estadoBomba = "✅"
			}
			estadoVentilador := "❌"
			if estado.VentiladorActivo {
				estadoVentilador = "✅"
			}

			fmt.Printf("\n⏱️  T+%ds | 🌡️ %.1f°C | 💧 %.1f%% | 💨 %s | 🚿 %s | ☀️ %.0f%%\n",
				segundo, tempPromedio, humedadPromedio, estadoVentilador, estadoBomba, luzSolar)

			if estado.SistemaEmergencia {
				fmt.Printf("🚨 SISTEMA EN EMERGENCIA\n")
			}
		}

		time.Sleep(50 * time.Millisecond) // Simulación acelerada
	}

	fmt.Println("\n=== SIMULACIÓN COMPLETADA ===")

	// Estado final de actuadores
	fmt.Printf("\n🔧 ESTADO FINAL DE ACTUADORES:\n")
	fmt.Printf("  - Bomba de riego: %v\n", map[bool]string{true: "🟢 ACTIVA", false: "🔴 INACTIVA"}[estado.BombaActivada])
	fmt.Printf("  - Sistema de ventilación: %v\n", map[bool]string{true: "🟢 ACTIVO", false: "🔴 INACTIVO"}[estado.VentiladorActivo])
	fmt.Printf("  - Sistema de emergencia: %v\n", map[bool]string{true: "🟡 ACTIVADO", false: "🟢 NORMAL"}[estado.SistemaEmergencia])

	// Análisis de últimas mediciones por zona
	fmt.Printf("\n📈 ESTADO FINAL POR ZONA:\n")
	for i := 1; i <= 8; i++ {
		nombreTemp := fmt.Sprintf("invernadero.zona%d.temperatura", i)
		nombreHumedad := fmt.Sprintf("invernadero.zona%d.humedad_suelo", i)

		ultimaTemp, errTemp := manager.ConsultarUltimoPunto(nombreTemp)
		ultimaHumedad, errHum := manager.ConsultarUltimoPunto(nombreHumedad)

		if errTemp == nil && errHum == nil {
			temp, _ := ultimaTemp.Valor.(float64)
			hum, _ := ultimaHumedad.Valor.(float64)

			// Determinar estado de la zona
			estadoZona := "🟢 ÓPTIMA"
			if temp > 30 || hum < 25 {
				estadoZona = "🔴 CRÍTICA"
			} else if temp > 26 || hum < 35 {
				estadoZona = "🟡 ATENCIÓN"
			}

			fmt.Printf("  Zona %d: %.1f°C, %.1f%% %s\n", i, temp, hum, estadoZona)
		}
	}

	// Resumen de datos almacenados
	listaSeries, _ := manager.ListarSeries()
	fmt.Printf("\n💾 DATOS ALMACENADOS:\n")
	fmt.Printf("  - Series creadas: %d\n", len(listaSeries))

	totalPuntos := 0
	for _, serie := range listaSeries[:5] { // Mostrar solo las primeras 5 para no saturar
		horaInicio := time.Now().Add(-10 * time.Minute)
		horaFinal := time.Now()
		puntos, err := manager.ConsultarRango(serie, horaInicio, horaFinal)
		if err == nil {
			fmt.Printf("  - %s: %d puntos\n", serie, len(puntos))
			totalPuntos += len(puntos)
		}
	}
	fmt.Printf("  - Puntos totales estimados: %d+\n", totalPuntos)

	// Reglas configuradas
	reglasConfigured := manager.ListarReglas()
	fmt.Printf("\n⚙️  REGLAS CONFIGURADAS:\n")
	for id, regla := range reglasConfigured {
		estadoRegla := "🟢 ACTIVA"
		if !regla.Activa {
			estadoRegla = "🔴 INACTIVA"
		}
		fmt.Printf("  - %s: %s %s\n", id, regla.Nombre, estadoRegla)
	}

	fmt.Println("\n🎉 SISTEMA DE AGRICULTURA INTELIGENTE FINALIZADO EXITOSAMENTE")
	fmt.Printf("   Tiempo total de simulación: 5 minutos (300 muestras por sensor)\n")
	fmt.Printf("   Sensores monitoreados: %d zonas + 5 sensores ambientales\n", 8)
	fmt.Printf("   Reglas de automatización: %d activas\n", len(reglasConfigured))
}
