package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cbiale/sensorwave/despachador"
	"github.com/cbiale/sensorwave/edge"
)

func main() {
	fmt.Println("=== Simulación de Sistema Distribuido ===")
	fmt.Println("Dos nodos Edge + Despachador + Cliente")
	fmt.Println()

	// Limpiar bases de datos anteriores
	os.RemoveAll("test_nodo1.db")
	os.RemoveAll("test_nodo2.db")
	os.RemoveAll("test_despachador.db")

	// 1. Crear dos nodos Edge
	fmt.Println("1. Creando nodos Edge...")
	nodo1, err := edge.Crear("test_nodo1.db")
	if err != nil {
		log.Fatal("Error creando nodo1:", err)
	}
	defer nodo1.Cerrar()

	nodo2, err := edge.Crear("test_nodo2.db")
	if err != nil {
		log.Fatal("Error creando nodo2:", err)
	}
	defer nodo2.Cerrar()

	// 2. Crear despachador
	fmt.Println("2. Creando despachador...")
	desp, err := despachador.CrearDespachador("test_despachador.db")
	if err != nil {
		log.Fatal("Error creando despachador:", err)
	}
	defer desp.Cerrar()

	// 3. Registrar nodos en el despachador
	fmt.Println("3. Registrando nodos en despachador...")
	err = desp.RegistrarNodo("nodo-edge-1", "localhost:8001")
	if err != nil {
		log.Fatal("Error registrando nodo1:", err)
	}

	err = desp.RegistrarNodo("nodo-edge-2", "localhost:8002")
	if err != nil {
		log.Fatal("Error registrando nodo2:", err)
	}

	// 4. Crear series en cada nodo (2 series por nodo)
	fmt.Println("4. Creando series en nodos...")

	// Series para nodo1
	serie1 := edge.Serie{
		NombreSerie:      "temperatura-sala1",
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.LZ4,
		CompresionBytes:  edge.DeltaDelta,
		TamañoBloque:     10,
	}

	serie2 := edge.Serie{
		NombreSerie:      "humedad-sala1",
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.ZSTD,
		CompresionBytes:  edge.RLE,
		TamañoBloque:     10,
	}

	// Series para nodo2
	serie3 := edge.Serie{
		NombreSerie:      "temperatura-sala2",
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.Snappy,
		CompresionBytes:  edge.Bits,
		TamañoBloque:     10,
	}

	serie4 := edge.Serie{
		NombreSerie:      "presion-atmosferica",
		TipoDatos:        edge.TipoNumerico,
		CompresionBloque: edge.Gzip,
		CompresionBytes:  edge.DeltaDelta,
		TamañoBloque:     10,
	}

	// Crear series a través del despachador
	if err := desp.CrearSerie("nodo-edge-1", serie1); err != nil {
		log.Fatal("Error creando serie1:", err)
	}
	if err := desp.CrearSerie("nodo-edge-1", serie2); err != nil {
		log.Fatal("Error creando serie2:", err)
	}
	if err := desp.CrearSerie("nodo-edge-2", serie3); err != nil {
		log.Fatal("Error creando serie3:", err)
	}
	if err := desp.CrearSerie("nodo-edge-2", serie4); err != nil {
		log.Fatal("Error creando serie4:", err)
	}

	// 5. Insertar datos en las series (5 datos por serie)
	fmt.Println("5. Insertando datos...")
	baseTime := time.Now().Add(-1 * time.Hour)

	// Datos para temperatura-sala1 (nodo1)
	for i := 0; i < 5; i++ {
		timestamp := baseTime.Add(time.Duration(i*10) * time.Minute).UnixNano()
		temp := 20.0 + float64(i)*0.5 // 20.0, 20.5, 21.0, 21.5, 22.0
		if err := desp.Insertar("nodo-edge-1", "temperatura-sala1", timestamp, temp); err != nil {
			log.Printf("Error insertando temperatura-sala1[%d]: %v", i, err)
		}
	}

	// Datos para humedad-sala1 (nodo1)
	for i := 0; i < 5; i++ {
		timestamp := baseTime.Add(time.Duration(i*10) * time.Minute).UnixNano()
		hum := 45.0 + float64(i)*2.0 // 45.0, 47.0, 49.0, 51.0, 53.0
		if err := desp.Insertar("nodo-edge-1", "humedad-sala1", timestamp, hum); err != nil {
			log.Printf("Error insertando humedad-sala1[%d]: %v", i, err)
		}
	}

	// Datos para temperatura-sala2 (nodo2)
	for i := 0; i < 5; i++ {
		timestamp := baseTime.Add(time.Duration(i*10) * time.Minute).UnixNano()
		temp := 18.0 + float64(i)*0.8 // 18.0, 18.8, 19.6, 20.4, 21.2
		if err := desp.Insertar("nodo-edge-2", "temperatura-sala2", timestamp, temp); err != nil {
			log.Printf("Error insertando temperatura-sala2[%d]: %v", i, err)
		}
	}

	// Datos para presion-atmosferica (nodo2)
	for i := 0; i < 5; i++ {
		timestamp := baseTime.Add(time.Duration(i*10) * time.Minute).UnixNano()
		pres := 1013.25 + float64(i)*0.1 // 1013.25, 1013.35, 1013.45, 1013.55, 1013.65
		if err := desp.Insertar("nodo-edge-2", "presion-atmosferica", timestamp, pres); err != nil {
			log.Printf("Error insertando presion-atmosferica[%d]: %v", i, err)
		}
	}

	// Esperar un poco para que se procesen los buffers
	fmt.Println("6. Esperando procesamiento de buffers...")
	time.Sleep(2 * time.Second)

	// 7. Cliente consulta una serie
	fmt.Println("7. Cliente consultando serie 'temperatura-sala1'...")

	// Consultar rango completo
	tiempoInicio := baseTime.Add(-10 * time.Minute)
	tiempoFin := time.Now()

	mediciones, err := desp.ConsultarRango("temperatura-sala1", tiempoInicio, tiempoFin)
	if err != nil {
		log.Printf("Error consultando temperatura-sala1: %v", err)
	} else {
		fmt.Printf("Mediciones encontradas: %d\n", len(mediciones))
		for i, m := range mediciones {
			timestamp := time.Unix(0, m.Tiempo)
			fmt.Printf("  [%d] %s: %.2f°C\n", i+1, timestamp.Format("15:04:05"), m.Valor)
		}
	}

	// 8. Consultar último punto
	fmt.Println("\n8. Consultando último punto de 'temperatura-sala1'...")
	ultimoPunto, err := desp.ConsultarUltimoPunto("temperatura-sala1")
	if err != nil {
		log.Printf("Error consultando último punto: %v", err)
	} else {
		timestamp := time.Unix(0, ultimoPunto.Tiempo)
		fmt.Printf("Último punto: %s: %.2f°C\n", timestamp.Format("15:04:05"), ultimoPunto.Valor)
	}

	// 9. Consultar primer punto
	fmt.Println("\n9. Consultando primer punto de 'temperatura-sala1'...")
	primerPunto, err := desp.ConsultarPrimerPunto("temperatura-sala1")
	if err != nil {
		log.Printf("Error consultando primer punto: %v", err)
	} else {
		timestamp := time.Unix(0, primerPunto.Tiempo)
		fmt.Printf("Primer punto: %s: %.2f°C\n", timestamp.Format("15:04:05"), primerPunto.Valor)
	}

	// 10. Mostrar información de nodos registrados
	fmt.Println("\n10. Información de nodos registrados:")
	nodos := desp.ListarNodos()
	for id, nodo := range nodos {
		fmt.Printf("Nodo: %s\n", id)
		fmt.Printf("  Dirección: %s\n", nodo.Direccion)
		fmt.Printf("  Activo: %v\n", nodo.Activo)
		fmt.Printf("  Series: %d\n", len(nodo.Series))
		for serie, tipo := range nodo.Series {
			fmt.Printf("    - %s (%s)\n", serie, tipo)
		}
		fmt.Println()
	}

	// 11. Mostrar series globales
	fmt.Println("11. Series disponibles en el sistema:")
	seriesGlobales := desp.ListarSeriesGlobal()
	for i, serie := range seriesGlobales {
		fmt.Printf("  [%d] %s\n", i+1, serie)
	}

	fmt.Println("\n=== Simulación completada exitosamente ===")
}
