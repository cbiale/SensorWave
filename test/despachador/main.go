package despachador

import (
	"fmt"
	"time"

	despachador "github.com/cbiale/sensorwave/despachador"
	edge "github.com/cbiale/sensorwave/edge"
)

func main() {
	// Se crea una instancia del ManagerDespachador
	managerDespachador, err := despachador.Crear("localhost", "4222")
	// Si hay un error al crear el despachador, se imprime el error y se termina la ejecución
	if err != nil {
		fmt.Println("Error al crear el despachador:", err)
		return
	}

	// Se crea una instancia del ManagerEdge
	managerEdge, err := edge.Crear("edge-1.db", "localhost", "4222")
	// Si hay un error al crear el manager edge, se imprime el error y se termina la ejecución
	if err != nil {
		fmt.Println("Error al crear el manager edge:", err)
		return
	}

	// Se crea una serie de datos para el sensor de temperatura
	managerEdge.CrearSerie(edge.Serie{
		Path : "sensor/temperatura",
		TipoDatos: edge.TipoNumerico,
		CompresionBytes: edge.RLE,
		CompresionBloque: edge.ZSTD,
	})

	// Se insertan datos
	for i := 0; i < 10; i++ {
		managerEdge.Insertar("sensor/temperatura", time.Now().Unix(), float64(20+i))
	}

	
}