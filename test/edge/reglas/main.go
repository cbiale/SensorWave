package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/cbiale/sensorwave/edge"
)

func main() {
	fmt.Println("=== TEST COMPLETO DE MOTOR DE REGLAS ===")

	// Crear ManagerEdge con motor de reglas integrado
	manager, err := edge.Crear("test_reglas.db")
	if err != nil {
		fmt.Printf("Error al crear ManagerEdge: %v\n", err)
		return
	}
	defer manager.Cerrar()
	fmt.Println("ManagerEdge y Motor de reglas activado")

	// Variables para rastrear ejecuciones de acciones
	contadorRiego := 0
	contadorVentilacion := 0
	contadorAlertas := 0
	contadorLogs := 0

	// Registrar ejecutores personalizados
	manager.RegistrarEjecutor("control_riego", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		contadorRiego++
		fmt.Printf("🚿 RIEGO ACTIVADO [%d]: %s - Regla: %s - Valores: %v\n",
			contadorRiego, accion.Destino, regla.Nombre, valores)
		return nil
	})

	manager.RegistrarEjecutor("activar_actuador", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		contadorVentilacion++
		fmt.Printf("💨 VENTILADOR ACTIVADO [%d]: %s - Valores: %v\n",
			contadorVentilacion, accion.Destino, valores)
		return nil
	})

	manager.RegistrarEjecutor("enviar_alerta", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		contadorAlertas++
		fmt.Printf("🚨 ALERTA [%d]: %s\n", contadorAlertas, accion.Destino)
		return nil
	})

	manager.RegistrarEjecutor("log", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		contadorLogs++
		fmt.Printf("📝 LOG [%d]: %s - Valores: %v\n", contadorLogs, accion.Destino, valores)
		return nil
	})

	manager.RegistrarEjecutor("control_climatizacion", func(accion edge.Accion, regla *edge.Regla, valores map[string]float64) error {
		fmt.Printf("🌡️ CLIMATIZACIÓN: %s - Temperatura: %.1f°C, Humedad: %.1f%%\n",
			accion.Destino, valores["sensor.temperatura"], valores["sensor.humedad"])
		return nil
	})

	// Test 1: Regla simple para temperatura alta
	fmt.Println("\n--- Test 1: Reglas individuales ---")

	reglaTemp := &edge.Regla{
		ID:     "temp_alta",
		Nombre: "Temperatura Alta",
		Condiciones: []edge.Condicion{{
			Serie:    "invernadero.temperatura",
			Operador: edge.OperadorMayorIgual,
			Valor:    30.0,
			VentanaT: 30 * time.Second,
		}},
		Acciones: []edge.Accion{
			{
				Tipo:    "activar_actuador",
				Destino: "ventilador_principal",
				Params:  map[string]string{"velocidad": "alta"},
			},
			{
				Tipo:    "enviar_alerta",
				Destino: "temperatura crítica detectada",
				Params:  map[string]string{"prioridad": "alta"},
			},
		},
		Logica: edge.LogicaAND,
	}

	// Test 2: Regla con promedio de múltiples sensores
	reglaHumedadPromedio := &edge.Regla{
		ID:     "humedad_baja_promedio",
		Nombre: "Control de Riego por Humedad Promedio",
		Condiciones: []edge.Condicion{{
			SeriesGrupo: []string{
				"invernadero.nodo1.humedad_suelo",
				"invernadero.nodo2.humedad_suelo",
				"invernadero.nodo3.humedad_suelo",
			},
			Agregacion: edge.AgregacionPromedio,
			Operador:   edge.OperadorMenorIgual,
			Valor:      25.0,
			VentanaT:   45 * time.Second,
		}},
		Acciones: []edge.Accion{{
			Tipo:    "control_riego",
			Destino: "zona_cultivo_principal",
			Params:  map[string]string{"duracion": "300s", "intensidad": "media"},
		}},
		Logica: edge.LogicaAND,
	}

	// Test 3: Regla compleja con múltiples condiciones AND
	reglaClimatizacion := &edge.Regla{
		ID:     "control_climatizacion",
		Nombre: "Control Automático de Climatización",
		Condiciones: []edge.Condicion{
			{
				Serie:    "sensor.temperatura",
				Operador: edge.OperadorMayorIgual,
				Valor:    22.0,
				VentanaT: 30 * time.Second,
			},
			{
				Serie:    "sensor.temperatura",
				Operador: edge.OperadorMenorIgual,
				Valor:    28.0,
				VentanaT: 30 * time.Second,
			},
			{
				Serie:    "sensor.humedad",
				Operador: edge.OperadorMayorIgual,
				Valor:    40.0,
				VentanaT: 30 * time.Second,
			},
			{
				Serie:    "sensor.humedad",
				Operador: edge.OperadorMenorIgual,
				Valor:    70.0,
				VentanaT: 30 * time.Second,
			},
		},
		Acciones: []edge.Accion{{
			Tipo:    "control_climatizacion",
			Destino: "sistema_hvac",
			Params:  map[string]string{"modo": "optimo"},
		}},
		Logica: edge.LogicaAND,
	}

	// Test 4: Regla con lógica OR
	reglaEmergencia := &edge.Regla{
		ID:     "emergencia_ambiental",
		Nombre: "Condiciones de Emergencia",
		Condiciones: []edge.Condicion{
			{
				Serie:    "sensor.temperatura",
				Operador: edge.OperadorMayor,
				Valor:    40.0,
				VentanaT: 15 * time.Second,
			},
			{
				Serie:    "sensor.humedad",
				Operador: edge.OperadorMenor,
				Valor:    10.0,
				VentanaT: 15 * time.Second,
			},
		},
		Acciones: []edge.Accion{
			{
				Tipo:    "enviar_alerta",
				Destino: "EMERGENCIA: condiciones ambientales críticas",
				Params:  map[string]string{"prioridad": "critica"},
			},
			{
				Tipo:    "activar_actuador",
				Destino: "sistema_emergencia",
			},
		},
		Logica: edge.LogicaOR,
	}

	// Agregar reglas al motor
	fmt.Println("\n--- Agregando reglas al motor ---")

	if err := manager.AgregarRegla(reglaTemp); err != nil {
		log.Fatalf("Error agregando regla de temperatura: %v", err)
	}
	fmt.Printf("✓ Regla agregada: %s\n", reglaTemp.Nombre)

	if err := manager.AgregarRegla(reglaHumedadPromedio); err != nil {
		log.Fatalf("Error agregando regla de humedad: %v", err)
	}
	fmt.Printf("✓ Regla agregada: %s\n", reglaHumedadPromedio.Nombre)

	if err := manager.AgregarRegla(reglaClimatizacion); err != nil {
		log.Fatalf("Error agregando regla de climatización: %v", err)
	}
	fmt.Printf("✓ Regla agregada: %s\n", reglaClimatizacion.Nombre)

	if err := manager.AgregarRegla(reglaEmergencia); err != nil {
		log.Fatalf("Error agregando regla de emergencia: %v", err)
	}
	fmt.Printf("✓ Regla agregada: %s\n", reglaEmergencia.Nombre)

	fmt.Println("Reglas agregadas exitosamente")

	// Test 5: Simulación completa de escenarios
	fmt.Println("\n--- Test 5: Simulación de escenarios ---")

	ahora := time.Now()
	rand.Seed(ahora.UnixNano())

	// Escenario 1: Condiciones normales
	fmt.Println("Escenario 1: Condiciones ambientales normales")
	for i := 0; i < 10; i++ {
		temp := 24.0 + rand.Float64()*2     // 24-26°C
		humedad := 55.0 + rand.Float64()*10 // 55-65%

		timestamp := ahora.Add(time.Duration(i) * 3 * time.Second)
		manager.ProcesarDatoRegla("sensor.temperatura", temp, timestamp)
		manager.ProcesarDatoRegla("sensor.humedad", humedad, timestamp)

		time.Sleep(50 * time.Millisecond)
	}

	// Escenario 2: Temperatura alta (activar ventilación)
	fmt.Println("\nEscenario 2: Temperatura alta - debe activar ventilación")
	for i := 0; i < 8; i++ {
		temp := 31.0 + rand.Float64()*3 // 31-34°C
		timestamp := ahora.Add(40*time.Second + time.Duration(i)*2*time.Second)
		manager.ProcesarDatoRegla("invernadero.temperatura", temp, timestamp)
		time.Sleep(50 * time.Millisecond)
	}

	// Escenario 3: Humedad del suelo baja (activar riego)
	fmt.Println("\nEscenario 3: Humedad del suelo baja - debe activar riego")
	for i := 0; i < 12; i++ {
		// Simular 3 nodos con humedad baja
		humedad1 := 15.0 + rand.Float64()*8 // 15-23%
		humedad2 := 18.0 + rand.Float64()*6 // 18-24%
		humedad3 := 16.0 + rand.Float64()*7 // 16-23%

		timestamp := ahora.Add(70*time.Second + time.Duration(i)*3*time.Second)
		manager.ProcesarDatoRegla("invernadero.nodo1.humedad_suelo", humedad1, timestamp)
		manager.ProcesarDatoRegla("invernadero.nodo2.humedad_suelo", humedad2, timestamp)
		manager.ProcesarDatoRegla("invernadero.nodo3.humedad_suelo", humedad3, timestamp)
		time.Sleep(50 * time.Millisecond)
	}

	// Escenario 4: Condiciones óptimas (activar climatización)
	fmt.Println("\nEscenario 4: Condiciones óptimas - debe activar climatización")
	for i := 0; i < 8; i++ {
		temp := 24.0 + rand.Float64()*2     // 24-26°C (en rango óptimo)
		humedad := 50.0 + rand.Float64()*15 // 50-65% (en rango óptimo)

		timestamp := ahora.Add(120*time.Second + time.Duration(i)*2*time.Second)
		manager.ProcesarDatoRegla("sensor.temperatura", temp, timestamp)
		manager.ProcesarDatoRegla("sensor.humedad", humedad, timestamp)
		time.Sleep(50 * time.Millisecond)
	}

	// Escenario 5: Condiciones de emergencia
	fmt.Println("\nEscenario 5: Emergencia - temperatura crítica")
	for i := 0; i < 5; i++ {
		temp := 42.0 + rand.Float64()*3 // 42-45°C (crítica)
		timestamp := ahora.Add(150*time.Second + time.Duration(i)*1*time.Second)
		manager.ProcesarDatoRegla("sensor.temperatura", temp, timestamp)
		time.Sleep(50 * time.Millisecond)
	}

	// Escenario 6: Emergencia - humedad crítica baja
	fmt.Println("\nEscenario 6: Emergencia - humedad crítica baja")
	for i := 0; i < 5; i++ {
		humedad := 5.0 + rand.Float64()*4 // 5-9% (crítica)
		timestamp := ahora.Add(170*time.Second + time.Duration(i)*1*time.Second)
		manager.ProcesarDatoRegla("sensor.humedad", humedad, timestamp)
		time.Sleep(50 * time.Millisecond)
	}

	// Test 6: Gestión dinámica de reglas
	fmt.Println("\n--- Test 6: Gestión dinámica de reglas ---")

	reglas := manager.ListarReglas()
	fmt.Printf("Reglas configuradas (%d):\n", len(reglas))
	for id, regla := range reglas {
		fmt.Printf("  - %s: %s (Activa: %t)\n", id, regla.Nombre, regla.Activa)
	}

	// Test actualización de regla
	fmt.Println("\nActualizando umbral de temperatura crítica (30°C → 35°C)...")
	reglaTemp.Condiciones[0].Valor = 35.0 // Cambiar umbral
	if err := manager.ActualizarRegla(reglaTemp); err != nil {
		fmt.Printf("Error actualizando regla: %v\n", err)
	} else {
		fmt.Println("✓ Regla actualizada exitosamente")
	}

	// Probar con temperatura que ahora no debe activar la regla
	fmt.Println("\nProbando temperatura de 32°C (no debe activar regla con nuevo umbral)...")
	manager.ProcesarDatoRegla("invernadero.temperatura", 32.0, ahora.Add(200*time.Second))
	time.Sleep(100 * time.Millisecond)

	// Test 7: Control de habilitación del motor
	fmt.Println("\n--- Test 7: Control de habilitación ---")

	fmt.Println("Deshabilitando motor temporalmente...")
	manager.HabilitarMotorReglas(false)
	fmt.Println("Enviando datos con motor deshabilitado (no debe ejecutar acciones):")
	manager.ProcesarDatoRegla("invernadero.temperatura", 40.0, ahora.Add(210*time.Second))
	manager.ProcesarDatoRegla("sensor.temperatura", 45.0, ahora.Add(211*time.Second))
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\nRehabilitando motor...")
	manager.HabilitarMotorReglas(true)
	fmt.Println("Enviando datos con motor habilitado:")
	manager.ProcesarDatoRegla("invernadero.temperatura", 40.0, ahora.Add(220*time.Second))
	time.Sleep(100 * time.Millisecond)

	// Test 8: Eliminación de reglas
	fmt.Println("\n--- Test 8: Eliminación de reglas ---")

	fmt.Println("Eliminando regla de climatización...")
	if err := manager.EliminarRegla("control_climatizacion"); err != nil {
		fmt.Printf("Error eliminando regla: %v\n", err)
	} else {
		fmt.Println("✓ Regla eliminada exitosamente")
	}

	reglasFinales := manager.ListarReglas()
	fmt.Printf("Reglas restantes: %d\n", len(reglasFinales))
	for id, regla := range reglasFinales {
		fmt.Printf("  - %s: %s\n", id, regla.Nombre)
	}

}
