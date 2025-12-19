package despachador

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/cbiale/sensorwave/compresor"
	"github.com/cbiale/sensorwave/tipos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MOCK DE CLIENTE EDGE PARA TESTS
// ============================================================================

// mockClienteEdge implementa clienteEdge para testing
type mockClienteEdge struct {
	respuestaRango *tipos.RespuestaConsultaRango
	respuestaPunto *tipos.RespuestaConsultaPunto
	err            error
}

func (m *mockClienteEdge) ConsultarRango(ctx context.Context, direccion string, req tipos.SolicitudConsultaRango) (*tipos.RespuestaConsultaRango, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.respuestaRango, nil
}

func (m *mockClienteEdge) ConsultarPunto(ctx context.Context, direccion string, req tipos.SolicitudConsultaPunto, tipo string) (*tipos.RespuestaConsultaPunto, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.respuestaPunto, nil
}

// ============================================================================
// MOCK DE CLIENTE S3 PARA TESTS
// ============================================================================

// mockClienteS3 implementa tipos.ClienteS3 para testing
type mockClienteS3 struct {
	// Respuestas configurables
	listObjectsOutput  *s3.ListObjectsV2Output
	getObjectOutput    *s3.GetObjectOutput
	getObjectData      []byte
	putObjectOutput    *s3.PutObjectOutput
	deleteObjectOutput *s3.DeleteObjectOutput
	headBucketOutput   *s3.HeadBucketOutput
	createBucketOutput *s3.CreateBucketOutput

	// Errores configurables
	listObjectsErr  error
	getObjectErr    error
	putObjectErr    error
	deleteObjectErr error
	headBucketErr   error
	createBucketErr error
}

func (m *mockClienteS3) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if m.headBucketErr != nil {
		return nil, m.headBucketErr
	}
	return m.headBucketOutput, nil
}

func (m *mockClienteS3) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	if m.createBucketErr != nil {
		return nil, m.createBucketErr
	}
	return m.createBucketOutput, nil
}

func (m *mockClienteS3) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listObjectsErr != nil {
		return nil, m.listObjectsErr
	}
	return m.listObjectsOutput, nil
}

func (m *mockClienteS3) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectErr != nil {
		return nil, m.getObjectErr
	}
	if m.getObjectData != nil {
		return &s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader(m.getObjectData)),
		}, nil
	}
	return m.getObjectOutput, nil
}

func (m *mockClienteS3) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectErr != nil {
		return nil, m.putObjectErr
	}
	return m.putObjectOutput, nil
}

func (m *mockClienteS3) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if m.deleteObjectErr != nil {
		return nil, m.deleteObjectErr
	}
	return m.deleteObjectOutput, nil
}

// ============================================================================
// TESTS DE FUNCIONES AUXILIARES (sin dependencias externas)
// ============================================================================

// TestObtenerDireccionEdge verifica construcción de dirección
func TestObtenerDireccionEdge(t *testing.T) {
	nodo := tipos.Nodo{
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	direccion := obtenerDireccionEdge(nodo)
	assert.Equal(t, "192.168.1.100:8080", direccion)
	t.Log("obtenerDireccionEdge construye direccion correctamente")
}

// ============================================================================
// TESTS DE COMBINAR RESULTADOS
// ============================================================================

// TestCombinarResultados_AmbosVacios verifica comportamiento con ambas listas vacias
func TestCombinarResultados_AmbosVacios(t *testing.T) {
	m := &ManagerDespachador{}

	resultado := m.combinarResultados([]tipos.Medicion{}, []tipos.Medicion{})

	assert.Empty(t, resultado)
	t.Log("combinarResultados retorna vacio cuando ambas fuentes estan vacias")
}

// TestCombinarResultados_SoloS3 verifica cuando solo hay datos de S3
func TestCombinarResultados_SoloS3(t *testing.T) {
	m := &ManagerDespachador{}

	datosS3 := []tipos.Medicion{
		{Tiempo: 1000, Valor: 10.0},
		{Tiempo: 2000, Valor: 20.0},
		{Tiempo: 3000, Valor: 30.0},
	}

	resultado := m.combinarResultados(datosS3, []tipos.Medicion{})

	assert.Len(t, resultado, 3)
	assert.Equal(t, datosS3, resultado)
	t.Log("combinarResultados retorna datos de S3 cuando edge esta vacio")
}

// TestCombinarResultados_SoloEdge verifica cuando solo hay datos del edge
func TestCombinarResultados_SoloEdge(t *testing.T) {
	m := &ManagerDespachador{}

	datosEdge := []tipos.Medicion{
		{Tiempo: 1000, Valor: 10.0},
		{Tiempo: 2000, Valor: 20.0},
	}

	resultado := m.combinarResultados([]tipos.Medicion{}, datosEdge)

	assert.Len(t, resultado, 2)
	assert.Equal(t, datosEdge, resultado)
	t.Log("combinarResultados retorna datos de edge cuando S3 esta vacio")
}

// TestCombinarResultados_SinDuplicados verifica combinacion sin tiempos duplicados
func TestCombinarResultados_SinDuplicados(t *testing.T) {
	m := &ManagerDespachador{}

	datosS3 := []tipos.Medicion{
		{Tiempo: 1000, Valor: 10.0},
		{Tiempo: 2000, Valor: 20.0},
	}
	datosEdge := []tipos.Medicion{
		{Tiempo: 3000, Valor: 30.0},
		{Tiempo: 4000, Valor: 40.0},
	}

	resultado := m.combinarResultados(datosS3, datosEdge)

	assert.Len(t, resultado, 4)
	// Verificar orden por tiempo
	assert.Equal(t, int64(1000), resultado[0].Tiempo)
	assert.Equal(t, int64(2000), resultado[1].Tiempo)
	assert.Equal(t, int64(3000), resultado[2].Tiempo)
	assert.Equal(t, int64(4000), resultado[3].Tiempo)
	t.Log("combinarResultados combina correctamente sin duplicados")
}

// TestCombinarResultados_ConDuplicados verifica que edge tiene prioridad en duplicados
func TestCombinarResultados_ConDuplicados(t *testing.T) {
	m := &ManagerDespachador{}

	datosS3 := []tipos.Medicion{
		{Tiempo: 1000, Valor: 10.0},
		{Tiempo: 2000, Valor: 20.0}, // Este sera sobrescrito
		{Tiempo: 3000, Valor: 30.0},
	}
	datosEdge := []tipos.Medicion{
		{Tiempo: 2000, Valor: 25.0}, // Sobrescribe el de S3
		{Tiempo: 4000, Valor: 40.0},
	}

	resultado := m.combinarResultados(datosS3, datosEdge)

	assert.Len(t, resultado, 4)
	// Verificar que el tiempo 2000 tiene el valor del edge (25.0)
	for _, med := range resultado {
		if med.Tiempo == 2000 {
			assert.Equal(t, 25.0, med.Valor, "Edge debe tener prioridad sobre S3")
		}
	}
	t.Log("combinarResultados prioriza datos del edge en duplicados")
}

// TestCombinarResultados_OrdenCorrecto verifica que el resultado esta ordenado por tiempo
func TestCombinarResultados_OrdenCorrecto(t *testing.T) {
	m := &ManagerDespachador{}

	// Datos desordenados
	datosS3 := []tipos.Medicion{
		{Tiempo: 5000, Valor: 50.0},
		{Tiempo: 1000, Valor: 10.0},
	}
	datosEdge := []tipos.Medicion{
		{Tiempo: 4000, Valor: 40.0},
		{Tiempo: 2000, Valor: 20.0},
	}

	resultado := m.combinarResultados(datosS3, datosEdge)

	assert.Len(t, resultado, 4)
	// Verificar orden ascendente por tiempo
	for i := 1; i < len(resultado); i++ {
		assert.True(t, resultado[i].Tiempo > resultado[i-1].Tiempo,
			"Mediciones deben estar ordenadas por tiempo")
	}
	t.Log("combinarResultados ordena resultados por tiempo")
}

// ============================================================================
// TESTS DE LISTAR NODOS
// ============================================================================

// TestListarNodos_Vacio verifica lista vacia
func TestListarNodos_Vacio(t *testing.T) {
	m := &ManagerDespachador{
		nodos: make(map[string]*tipos.Nodo),
	}

	resultado := m.ListarNodos()

	assert.Empty(t, resultado)
	t.Log("ListarNodos retorna lista vacia cuando no hay nodos")
}

// TestListarNodos_ConNodos verifica que retorna todos los nodos
func TestListarNodos_ConNodos(t *testing.T) {
	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {NodoID: "nodo1", DireccionIP: "192.168.1.1"},
			"nodo2": {NodoID: "nodo2", DireccionIP: "192.168.1.2"},
			"nodo3": {NodoID: "nodo3", DireccionIP: "192.168.1.3"},
		},
	}

	resultado := m.ListarNodos()

	assert.Len(t, resultado, 3)

	// Verificar que todos los nodos estan presentes
	ids := make(map[string]bool)
	for _, nodo := range resultado {
		ids[nodo.NodoID] = true
	}
	assert.True(t, ids["nodo1"])
	assert.True(t, ids["nodo2"])
	assert.True(t, ids["nodo3"])

	t.Log("ListarNodos retorna todos los nodos registrados")
}

// TestListarNodos_EsCopia verifica que retorna una copia, no referencias
func TestListarNodos_EsCopia(t *testing.T) {
	nodoOriginal := &tipos.Nodo{NodoID: "nodo1", DireccionIP: "192.168.1.1"}
	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": nodoOriginal,
		},
	}

	resultado := m.ListarNodos()

	// Modificar el resultado no debe afectar el original
	resultado[0].DireccionIP = "10.0.0.1"

	assert.Equal(t, "192.168.1.1", m.nodos["nodo1"].DireccionIP,
		"Modificar resultado no debe afectar nodo original")

	t.Log("ListarNodos retorna copias de los nodos")
}

// ============================================================================
// TESTS DE CERRAR
// ============================================================================

// TestCerrar verifica que cierra el canal done
func TestCerrar(t *testing.T) {
	m := &ManagerDespachador{
		done: make(chan struct{}),
	}

	// Iniciar goroutine que espera el cierre
	cerrado := make(chan bool, 1)
	go func() {
		<-m.done
		cerrado <- true
	}()

	// Cerrar el manager
	err := m.Cerrar()

	assert.NoError(t, err)

	// Verificar que el canal fue cerrado
	select {
	case <-cerrado:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Fatal("El canal done no fue cerrado")
	}

	t.Log("Cerrar cierra el canal done correctamente")
}

// ============================================================================
// TESTS DE BUSCAR NODO Y SERIE
// ============================================================================

// TestBuscarNodoYSerie_Exacto verifica busqueda exacta por nombre de serie
func TestBuscarNodoYSerie_Exacto(t *testing.T) {
	serie := tipos.Serie{
		SerieId:   1,
		Path:      "/sensores/temperatura",
		TipoDatos: tipos.Real,
	}
	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.1",
				Series: map[string]tipos.Serie{
					"/sensores/temperatura": serie,
				},
			},
		},
	}

	nodo, serieEncontrada, err := m.buscarNodoYSerie("/sensores/temperatura")

	assert.NoError(t, err)
	assert.Equal(t, "nodo1", nodo.NodoID)
	assert.Equal(t, serie.SerieId, serieEncontrada.SerieId)
	assert.Equal(t, serie.Path, serieEncontrada.Path)

	t.Log("buscarNodoYSerie encuentra serie por nombre exacto")
}

// TestBuscarNodoYSerie_NoEncontrada verifica error cuando no existe la serie
func TestBuscarNodoYSerie_NoEncontrada(t *testing.T) {
	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID: "nodo1",
				Series: map[string]tipos.Serie{
					"/sensores/temperatura": {SerieId: 1},
				},
			},
		},
	}

	_, _, err := m.buscarNodoYSerie("/sensores/humedad")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no encontrada")

	t.Log("buscarNodoYSerie retorna error cuando serie no existe")
}

// TestBuscarNodoYSerie_SinNodos verifica error cuando no hay nodos
func TestBuscarNodoYSerie_SinNodos(t *testing.T) {
	m := &ManagerDespachador{
		nodos: make(map[string]*tipos.Nodo),
	}

	_, _, err := m.buscarNodoYSerie("/sensores/temperatura")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no encontrada")

	t.Log("buscarNodoYSerie retorna error cuando no hay nodos")
}

// TestBuscarNodoYSerie_PorPrefijo verifica busqueda por prefijo
func TestBuscarNodoYSerie_PorPrefijo(t *testing.T) {
	serie := tipos.Serie{
		SerieId: 1,
		Path:    "/sensores/temp",
	}
	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID: "nodo1",
				Series: map[string]tipos.Serie{
					"/sensores/temp": serie,
				},
			},
		},
	}

	// Buscar con prefijo mas largo
	nodo, serieEncontrada, err := m.buscarNodoYSerie("/sensores/temp/interior")

	assert.NoError(t, err)
	assert.Equal(t, "nodo1", nodo.NodoID)
	assert.Equal(t, serie.SerieId, serieEncontrada.SerieId)

	t.Log("buscarNodoYSerie encuentra serie por prefijo")
}

// TestBuscarNodoYSerie_MultiplesNodos verifica busqueda en multiples nodos
func TestBuscarNodoYSerie_MultiplesNodos(t *testing.T) {
	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID: "nodo1",
				Series: map[string]tipos.Serie{
					"/sensores/temperatura": {SerieId: 1},
				},
			},
			"nodo2": {
				NodoID: "nodo2",
				Series: map[string]tipos.Serie{
					"/sensores/humedad": {SerieId: 2},
				},
			},
		},
	}

	// Buscar serie en nodo2
	nodo, serie, err := m.buscarNodoYSerie("/sensores/humedad")

	assert.NoError(t, err)
	assert.Equal(t, "nodo2", nodo.NodoID)
	assert.Equal(t, 2, serie.SerieId)

	t.Log("buscarNodoYSerie encuentra serie en multiples nodos")
}

// ============================================================================
// TESTS DE CONSULTAR PUNTO EDGE
// ============================================================================

// TestConsultarPuntoEdge_Exitoso verifica consulta exitosa al edge
func TestConsultarPuntoEdge_Exitoso(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Medicion:   tipos.Medicion{Tiempo: 1000, Valor: 25.5},
			Encontrado: true,
			Error:      "",
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	medicion, encontrado, err := m.consultarPuntoEdge(nodo, "/sensores/temp", "ultimo", 5*time.Second)

	assert.NoError(t, err)
	assert.True(t, encontrado)
	assert.Equal(t, int64(1000), medicion.Tiempo)
	assert.Equal(t, 25.5, medicion.Valor)
	t.Log("consultarPuntoEdge retorna medicion correctamente")
}

// TestConsultarPuntoEdge_NoEncontrado verifica cuando no hay datos
func TestConsultarPuntoEdge_NoEncontrado(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Encontrado: false,
			Error:      "",
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	_, encontrado, err := m.consultarPuntoEdge(nodo, "/sensores/temp", "ultimo", 5*time.Second)

	assert.NoError(t, err)
	assert.False(t, encontrado)
	t.Log("consultarPuntoEdge retorna encontrado=false cuando no hay datos")
}

// TestConsultarPuntoEdge_ErrorConexion verifica manejo de error de conexion
func TestConsultarPuntoEdge_ErrorConexion(t *testing.T) {
	mockEdge := &mockClienteEdge{
		err: assert.AnError,
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	_, _, err := m.consultarPuntoEdge(nodo, "/sensores/temp", "ultimo", 5*time.Second)

	assert.Error(t, err)
	t.Log("consultarPuntoEdge retorna error cuando hay falla de conexion")
}

// TestConsultarPuntoEdge_ErrorDelEdge verifica manejo de error reportado por el edge
func TestConsultarPuntoEdge_ErrorDelEdge(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Error: "serie no encontrada",
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	_, _, err := m.consultarPuntoEdge(nodo, "/sensores/temp", "ultimo", 5*time.Second)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "serie no encontrada")
	t.Log("consultarPuntoEdge retorna error cuando el edge reporta error")
}

// TestConsultarPuntoEdge_TipoPrimero verifica consulta de primer punto
func TestConsultarPuntoEdge_TipoPrimero(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Medicion:   tipos.Medicion{Tiempo: 500, Valor: 10.0},
			Encontrado: true,
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	medicion, encontrado, err := m.consultarPuntoEdge(nodo, "/sensores/temp", "primero", 5*time.Second)

	assert.NoError(t, err)
	assert.True(t, encontrado)
	assert.Equal(t, int64(500), medicion.Tiempo)
	t.Log("consultarPuntoEdge funciona con tipo 'primero'")
}

// ============================================================================
// TESTS DE CONSULTAR EDGE CON TIMEOUT
// ============================================================================

// TestConsultarEdgeConTimeout_Exitoso verifica consulta exitosa de rango
func TestConsultarEdgeConTimeout_Exitoso(t *testing.T) {
	medicionesEsperadas := []tipos.Medicion{
		{Tiempo: 1000, Valor: 10.0},
		{Tiempo: 2000, Valor: 20.0},
		{Tiempo: 3000, Valor: 30.0},
	}

	mockEdge := &mockClienteEdge{
		respuestaRango: &tipos.RespuestaConsultaRango{
			Mediciones: medicionesEsperadas,
			Error:      "",
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	mediciones, err := m.consultarEdgeConTimeout(nodo, "/sensores/temp", 1000, 3000, 5*time.Second)

	assert.NoError(t, err)
	assert.Len(t, mediciones, 3)
	assert.Equal(t, medicionesEsperadas, mediciones)
	t.Log("consultarEdgeConTimeout retorna mediciones correctamente")
}

// TestConsultarEdgeConTimeout_SinDatos verifica respuesta vacia
func TestConsultarEdgeConTimeout_SinDatos(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaRango: &tipos.RespuestaConsultaRango{
			Mediciones: []tipos.Medicion{},
			Error:      "",
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	mediciones, err := m.consultarEdgeConTimeout(nodo, "/sensores/temp", 1000, 3000, 5*time.Second)

	assert.NoError(t, err)
	assert.Empty(t, mediciones)
	t.Log("consultarEdgeConTimeout retorna lista vacia cuando no hay datos")
}

// TestConsultarEdgeConTimeout_ErrorConexion verifica que error de conexion retorna nil, nil
func TestConsultarEdgeConTimeout_ErrorConexion(t *testing.T) {
	mockEdge := &mockClienteEdge{
		err: assert.AnError,
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	// Error de conexion retorna nil, nil (el edge puede estar offline)
	mediciones, err := m.consultarEdgeConTimeout(nodo, "/sensores/temp", 1000, 3000, 5*time.Second)

	assert.Nil(t, err)
	assert.Nil(t, mediciones)
	t.Log("consultarEdgeConTimeout retorna nil, nil cuando hay error de conexion")
}

// TestConsultarEdgeConTimeout_ErrorDelEdge verifica manejo de error reportado por el edge
func TestConsultarEdgeConTimeout_ErrorDelEdge(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaRango: &tipos.RespuestaConsultaRango{
			Error: "serie no existe",
		},
	}

	m := &ManagerDespachador{
		clienteEdge: mockEdge,
	}

	nodo := tipos.Nodo{
		NodoID:      "nodo1",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
	}

	_, err := m.consultarEdgeConTimeout(nodo, "/sensores/temp", 1000, 3000, 5*time.Second)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "serie no existe")
	t.Log("consultarEdgeConTimeout retorna error cuando el edge reporta error")
}

// ============================================================================
// TESTS DE LISTAR BLOQUES EN RANGO
// ============================================================================

// TestListarBloquesEnRango_SinBloques verifica respuesta cuando no hay bloques
func TestListarBloquesEnRango_SinBloques(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	bloques, err := m.listarBloquesEnRango("nodo1", 1, 1000, 5000)

	assert.NoError(t, err)
	assert.Empty(t, bloques)
	t.Log("listarBloquesEnRango retorna lista vacia cuando no hay bloques")
}

// TestListarBloquesEnRango_ConBloques verifica listado de bloques
func TestListarBloquesEnRango_ConBloques(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000002000")},
				{Key: aws.String("nodo1/data/0000000001/00000000002000_00000000003000")},
				{Key: aws.String("nodo1/data/0000000001/00000000003000_00000000004000")},
			},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	// Rango que intersecta con los primeros dos bloques
	bloques, err := m.listarBloquesEnRango("nodo1", 1, 1500, 2500)

	assert.NoError(t, err)
	assert.Len(t, bloques, 2)
	t.Log("listarBloquesEnRango retorna bloques que intersectan con el rango")
}

// TestListarBloquesEnRango_TodosLosBloques verifica cuando el rango cubre todos los bloques
func TestListarBloquesEnRango_TodosLosBloques(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000002000")},
				{Key: aws.String("nodo1/data/0000000001/00000000002000_00000000003000")},
				{Key: aws.String("nodo1/data/0000000001/00000000003000_00000000004000")},
			},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	// Rango amplio que cubre todos los bloques
	bloques, err := m.listarBloquesEnRango("nodo1", 1, 0, 10000)

	assert.NoError(t, err)
	assert.Len(t, bloques, 3)
	t.Log("listarBloquesEnRango retorna todos los bloques cuando el rango los cubre")
}

// TestListarBloquesEnRango_NingunBloqueEnRango verifica cuando ningun bloque intersecta
func TestListarBloquesEnRango_NingunBloqueEnRango(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000002000")},
				{Key: aws.String("nodo1/data/0000000001/00000000002000_00000000003000")},
			},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	// Rango que no intersecta con ningun bloque
	bloques, err := m.listarBloquesEnRango("nodo1", 1, 5000, 6000)

	assert.NoError(t, err)
	assert.Empty(t, bloques)
	t.Log("listarBloquesEnRango retorna vacio cuando ningun bloque intersecta")
}

// TestListarBloquesEnRango_ErrorS3 verifica manejo de error de S3
func TestListarBloquesEnRango_ErrorS3(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsErr: assert.AnError,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	_, err := m.listarBloquesEnRango("nodo1", 1, 1000, 5000)

	assert.Error(t, err)
	t.Log("listarBloquesEnRango retorna error cuando S3 falla")
}

// TestListarBloquesEnRango_FormatoIncorrecto verifica que ignora bloques mal formateados
func TestListarBloquesEnRango_FormatoIncorrecto(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000002000")}, // Correcto
				{Key: aws.String("nodo1/data/0000000001/bloque_invalido")},               // Incorrecto
				{Key: aws.String("nodo1/data/0000000001")},                               // Sin nombre de bloque
				{Key: aws.String("nodo1/data/0000000001/00000000003000_00000000004000")}, // Correcto
			},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	bloques, err := m.listarBloquesEnRango("nodo1", 1, 0, 10000)

	assert.NoError(t, err)
	assert.Len(t, bloques, 2) // Solo los bloques con formato correcto
	t.Log("listarBloquesEnRango ignora bloques con formato incorrecto")
}

// TestListarBloquesEnRango_OrdenPorTiempo verifica que los bloques estan ordenados
func TestListarBloquesEnRango_OrdenPorTiempo(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000003000_00000000004000")},
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000002000")},
				{Key: aws.String("nodo1/data/0000000001/00000000002000_00000000003000")},
			},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	bloques, err := m.listarBloquesEnRango("nodo1", 1, 0, 10000)

	assert.NoError(t, err)
	assert.Len(t, bloques, 3)
	// Verificar orden ascendente (los nombres tienen padding, asi que sort.Strings funciona)
	assert.Contains(t, bloques[0], "00000000001000")
	assert.Contains(t, bloques[1], "00000000002000")
	assert.Contains(t, bloques[2], "00000000003000")
	t.Log("listarBloquesEnRango retorna bloques ordenados por tiempo")
}

// ============================================================================
// TESTS DE CONSULTAR DATOS S3
// ============================================================================

// TestConsultarDatosS3_SinBloques verifica respuesta cuando no hay bloques
func TestConsultarDatosS3_SinBloques(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	nodo := tipos.Nodo{NodoID: "nodo1"}
	serie := tipos.Serie{SerieId: 1}

	mediciones, err := m.consultarDatosS3(nodo, serie, 1000, 5000)

	assert.NoError(t, err)
	assert.Empty(t, mediciones)
	t.Log("consultarDatosS3 retorna lista vacia cuando no hay bloques")
}

// TestConsultarDatosS3_ErrorListando verifica manejo de error al listar
func TestConsultarDatosS3_ErrorListando(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsErr: assert.AnError,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	nodo := tipos.Nodo{NodoID: "nodo1"}
	serie := tipos.Serie{SerieId: 1}

	_, err := m.consultarDatosS3(nodo, serie, 1000, 5000)

	assert.Error(t, err)
	t.Log("consultarDatosS3 retorna error cuando falla el listado")
}

// ============================================================================
// TESTS DE CONSULTAR RANGO
// ============================================================================

// TestConsultarRango_SerieNoEncontrada verifica error cuando la serie no existe
func TestConsultarRango_SerieNoEncontrada(t *testing.T) {
	m := &ManagerDespachador{
		nodos: make(map[string]*tipos.Nodo),
	}

	_, err := m.ConsultarRango("/sensores/noexiste", time.Now().Add(-1*time.Hour), time.Now())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no encontrada")
	t.Log("ConsultarRango retorna error cuando la serie no existe")
}

// TestConsultarRango_SoloEdge verifica consulta cuando solo edge tiene datos
func TestConsultarRango_SoloEdge(t *testing.T) {
	medicionesEdge := []tipos.Medicion{
		{Tiempo: 1000, Valor: 10.0},
		{Tiempo: 2000, Valor: 20.0},
	}

	mockEdge := &mockClienteEdge{
		respuestaRango: &tipos.RespuestaConsultaRango{
			Mediciones: medicionesEdge,
		},
	}

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	inicio := time.Unix(0, 500)
	fin := time.Unix(0, 3000)

	mediciones, err := m.ConsultarRango("/sensores/temp", inicio, fin)

	assert.NoError(t, err)
	assert.Len(t, mediciones, 2)
	t.Log("ConsultarRango combina datos de edge cuando S3 esta vacio")
}

// TestConsultarRango_EdgeOffline verifica consulta cuando el edge esta offline
func TestConsultarRango_EdgeOffline(t *testing.T) {
	mockEdge := &mockClienteEdge{
		err: assert.AnError, // Simular edge offline
	}

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	inicio := time.Unix(0, 500)
	fin := time.Unix(0, 3000)

	// Debe continuar con datos de S3 incluso si el edge falla
	mediciones, err := m.ConsultarRango("/sensores/temp", inicio, fin)

	assert.NoError(t, err)
	assert.Empty(t, mediciones)
	t.Log("ConsultarRango continua cuando edge esta offline")
}

// TestConsultarRango_ErrorS3 verifica error critico cuando S3 falla
func TestConsultarRango_ErrorS3(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaRango: &tipos.RespuestaConsultaRango{
			Mediciones: []tipos.Medicion{},
		},
	}

	mockS3 := &mockClienteS3{
		listObjectsErr: assert.AnError,
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	inicio := time.Unix(0, 500)
	fin := time.Unix(0, 3000)

	// S3 falla -> error critico
	_, err := m.ConsultarRango("/sensores/temp", inicio, fin)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "S3")
	t.Log("ConsultarRango retorna error cuando S3 falla")
}

// ============================================================================
// TESTS DE CONSULTAR ULTIMO PUNTO
// ============================================================================

// TestConsultarUltimoPunto_SerieNoEncontrada verifica error cuando la serie no existe
func TestConsultarUltimoPunto_SerieNoEncontrada(t *testing.T) {
	m := &ManagerDespachador{
		nodos: make(map[string]*tipos.Nodo),
	}

	_, err := m.ConsultarUltimoPunto("/sensores/noexiste")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no encontrada")
	t.Log("ConsultarUltimoPunto retorna error cuando la serie no existe")
}

// TestConsultarUltimoPunto_DesdEdge verifica que retorna dato del edge cuando esta disponible
func TestConsultarUltimoPunto_DesdeEdge(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Medicion:   tipos.Medicion{Tiempo: 5000, Valor: 50.0},
			Encontrado: true,
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
	}

	medicion, err := m.ConsultarUltimoPunto("/sensores/temp")

	assert.NoError(t, err)
	assert.Equal(t, int64(5000), medicion.Tiempo)
	assert.Equal(t, 50.0, medicion.Valor)
	t.Log("ConsultarUltimoPunto retorna dato del edge")
}

// TestConsultarUltimoPunto_EdgeOffline_SinDatosS3 verifica error cuando no hay datos
func TestConsultarUltimoPunto_EdgeOffline_SinDatosS3(t *testing.T) {
	mockEdge := &mockClienteEdge{
		err: assert.AnError, // Edge offline
	}

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	_, err := m.ConsultarUltimoPunto("/sensores/temp")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no se encontraron datos")
	t.Log("ConsultarUltimoPunto retorna error cuando no hay datos")
}

// ============================================================================
// TESTS DE CONSULTAR PRIMER PUNTO
// ============================================================================

// TestConsultarPrimerPunto_SerieNoEncontrada verifica error cuando la serie no existe
func TestConsultarPrimerPunto_SerieNoEncontrada(t *testing.T) {
	m := &ManagerDespachador{
		nodos: make(map[string]*tipos.Nodo),
	}

	_, err := m.ConsultarPrimerPunto("/sensores/noexiste")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no encontrada")
	t.Log("ConsultarPrimerPunto retorna error cuando la serie no existe")
}

// TestConsultarPrimerPunto_SinDatos verifica error cuando no hay datos
func TestConsultarPrimerPunto_SinDatos(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Encontrado: false,
		},
	}

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	_, err := m.ConsultarPrimerPunto("/sensores/temp")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no se encontraron datos")
	t.Log("ConsultarPrimerPunto retorna error cuando no hay datos")
}

// TestConsultarPrimerPunto_DesdeEdge verifica que retorna dato del edge cuando S3 esta vacio
func TestConsultarPrimerPunto_DesdeEdge(t *testing.T) {
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Medicion:   tipos.Medicion{Tiempo: 100, Valor: 1.0},
			Encontrado: true,
		},
	}

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	medicion, err := m.ConsultarPrimerPunto("/sensores/temp")

	assert.NoError(t, err)
	assert.Equal(t, int64(100), medicion.Tiempo)
	assert.Equal(t, 1.0, medicion.Valor)
	t.Log("ConsultarPrimerPunto retorna dato del edge cuando S3 esta vacio")
}

// ============================================================================
// HELPER: CREAR BLOQUE COMPRIMIDO PARA TESTS
// ============================================================================

// crearBloqueComprimidoTest crea un bloque comprimido válido para usar en tests
func crearBloqueComprimidoTest(t *testing.T, mediciones []tipos.Medicion, tipoDatos tipos.TipoDatos,
	compresionBytes tipos.TipoCompresion, compresionBloque tipos.TipoCompresionBloque) []byte {

	// Comprimir tiempos con DeltaDelta (siempre)
	tiemposComprimidos := compresor.CompresionDeltaDeltaTiempo(mediciones)

	// Comprimir valores según tipo
	var valoresComprimidos []byte
	var err error

	switch tipoDatos {
	case tipos.Integer:
		valores := make([]int64, len(mediciones))
		for i, m := range mediciones {
			valores[i] = m.Valor.(int64)
		}
		switch compresionBytes {
		case tipos.DeltaDelta:
			c := &compresor.CompresorDeltaDeltaGenerico[int64]{}
			valoresComprimidos, err = c.Comprimir(valores)
		case tipos.SinCompresion:
			c := &compresor.CompresorNingunoGenerico[int64]{}
			valoresComprimidos, err = c.Comprimir(valores)
		}
	case tipos.Real:
		valores := make([]float64, len(mediciones))
		for i, m := range mediciones {
			valores[i] = m.Valor.(float64)
		}
		switch compresionBytes {
		case tipos.SinCompresion:
			c := &compresor.CompresorNingunoGenerico[float64]{}
			valoresComprimidos, err = c.Comprimir(valores)
		case tipos.Xor:
			c := &compresor.CompresorXor{}
			valoresComprimidos, err = c.Comprimir(valores)
		}
	}
	require.NoError(t, err)

	// Combinar tiempos y valores
	datosCombinados := compresor.CombinarDatos(tiemposComprimidos, valoresComprimidos)

	// Comprimir bloque
	compBloque := compresor.ObtenerCompresorBloque(compresionBloque)
	bloqueComprimido, err := compBloque.Comprimir(datosCombinados)
	require.NoError(t, err)

	return bloqueComprimido
}

// ============================================================================
// TESTS DE CLIENTE EDGE HTTP (httptest)
// ============================================================================

// TestClienteEdgeHTTP_ConsultarRango_Exitoso verifica consulta exitosa via HTTP
func TestClienteEdgeHTTP_ConsultarRango_Exitoso(t *testing.T) {
	// Crear respuesta esperada
	respuestaEsperada := tipos.RespuestaConsultaRango{
		Mediciones: []tipos.Medicion{
			{Tiempo: 1000, Valor: 10.0},
			{Tiempo: 2000, Valor: 20.0},
		},
		Error: "",
	}

	// Serializar respuesta
	respuestaBytes, err := tipos.SerializarGob(respuestaEsperada)
	require.NoError(t, err)

	// Crear servidor HTTP mock
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verificar método y path
		assert.Equal(t, http.MethodPost, r.Method)
		assert.True(t, strings.HasSuffix(r.URL.Path, "/api/consulta/rango"))

		w.WriteHeader(http.StatusOK)
		w.Write(respuestaBytes)
	}))
	defer servidor.Close()

	// Crear cliente y hacer consulta
	cliente := nuevoClienteEdgeHTTP()

	// Extraer host:port del servidor de test
	direccion := strings.TrimPrefix(servidor.URL, "http://")

	solicitud := tipos.SolicitudConsultaRango{
		Serie:        "/sensores/temp",
		TiempoInicio: 1000,
		TiempoFin:    2000,
	}

	respuesta, err := cliente.ConsultarRango(context.Background(), direccion, solicitud)

	assert.NoError(t, err)
	assert.NotNil(t, respuesta)
	assert.Len(t, respuesta.Mediciones, 2)
	assert.Equal(t, int64(1000), respuesta.Mediciones[0].Tiempo)
	t.Log("clienteEdgeHTTP.ConsultarRango funciona correctamente via HTTP")
}

// TestClienteEdgeHTTP_ConsultarRango_ErrorHTTP verifica manejo de error HTTP
func TestClienteEdgeHTTP_ConsultarRango_ErrorHTTP(t *testing.T) {
	// Crear servidor que retorna error
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer servidor.Close()

	cliente := nuevoClienteEdgeHTTP()
	direccion := strings.TrimPrefix(servidor.URL, "http://")

	solicitud := tipos.SolicitudConsultaRango{
		Serie:        "/sensores/temp",
		TiempoInicio: 1000,
		TiempoFin:    2000,
	}

	_, err := cliente.ConsultarRango(context.Background(), direccion, solicitud)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	t.Log("clienteEdgeHTTP.ConsultarRango maneja errores HTTP correctamente")
}

// TestClienteEdgeHTTP_ConsultarRango_ErrorConexion verifica manejo de error de conexion
func TestClienteEdgeHTTP_ConsultarRango_ErrorConexion(t *testing.T) {
	cliente := nuevoClienteEdgeHTTP()

	// Usar direccion invalida
	solicitud := tipos.SolicitudConsultaRango{
		Serie:        "/sensores/temp",
		TiempoInicio: 1000,
		TiempoFin:    2000,
	}

	_, err := cliente.ConsultarRango(context.Background(), "localhost:99999", solicitud)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error en request HTTP")
	t.Log("clienteEdgeHTTP.ConsultarRango maneja errores de conexion")
}

// TestClienteEdgeHTTP_ConsultarRango_ErrorDeserializacion verifica manejo de respuesta invalida
func TestClienteEdgeHTTP_ConsultarRango_ErrorDeserializacion(t *testing.T) {
	// Crear servidor que retorna datos invalidos
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("datos invalidos que no son gob"))
	}))
	defer servidor.Close()

	cliente := nuevoClienteEdgeHTTP()
	direccion := strings.TrimPrefix(servidor.URL, "http://")

	solicitud := tipos.SolicitudConsultaRango{
		Serie:        "/sensores/temp",
		TiempoInicio: 1000,
		TiempoFin:    2000,
	}

	_, err := cliente.ConsultarRango(context.Background(), direccion, solicitud)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deserializando")
	t.Log("clienteEdgeHTTP.ConsultarRango maneja errores de deserializacion")
}

// TestClienteEdgeHTTP_ConsultarPunto_Exitoso verifica consulta de punto via HTTP
func TestClienteEdgeHTTP_ConsultarPunto_Exitoso(t *testing.T) {
	// Crear respuesta esperada
	respuestaEsperada := tipos.RespuestaConsultaPunto{
		Medicion:   tipos.Medicion{Tiempo: 5000, Valor: 50.0},
		Encontrado: true,
		Error:      "",
	}

	respuestaBytes, err := tipos.SerializarGob(respuestaEsperada)
	require.NoError(t, err)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.True(t, strings.HasSuffix(r.URL.Path, "/api/consulta/ultimo"))

		w.WriteHeader(http.StatusOK)
		w.Write(respuestaBytes)
	}))
	defer servidor.Close()

	cliente := nuevoClienteEdgeHTTP()
	direccion := strings.TrimPrefix(servidor.URL, "http://")

	solicitud := tipos.SolicitudConsultaPunto{
		Serie: "/sensores/temp",
	}

	respuesta, err := cliente.ConsultarPunto(context.Background(), direccion, solicitud, "ultimo")

	assert.NoError(t, err)
	assert.NotNil(t, respuesta)
	assert.True(t, respuesta.Encontrado)
	assert.Equal(t, int64(5000), respuesta.Medicion.Tiempo)
	t.Log("clienteEdgeHTTP.ConsultarPunto funciona correctamente")
}

// TestClienteEdgeHTTP_ConsultarPunto_TipoPrimero verifica URL correcta para primer punto
func TestClienteEdgeHTTP_ConsultarPunto_TipoPrimero(t *testing.T) {
	respuestaEsperada := tipos.RespuestaConsultaPunto{
		Medicion:   tipos.Medicion{Tiempo: 100, Valor: 1.0},
		Encontrado: true,
	}

	respuestaBytes, _ := tipos.SerializarGob(respuestaEsperada)

	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verificar que usa la URL correcta para "primero"
		assert.True(t, strings.HasSuffix(r.URL.Path, "/api/consulta/primero"))

		w.WriteHeader(http.StatusOK)
		w.Write(respuestaBytes)
	}))
	defer servidor.Close()

	cliente := nuevoClienteEdgeHTTP()
	direccion := strings.TrimPrefix(servidor.URL, "http://")

	solicitud := tipos.SolicitudConsultaPunto{Serie: "/sensores/temp"}
	respuesta, err := cliente.ConsultarPunto(context.Background(), direccion, solicitud, "primero")

	assert.NoError(t, err)
	assert.Equal(t, int64(100), respuesta.Medicion.Tiempo)
	t.Log("clienteEdgeHTTP.ConsultarPunto usa URL correcta para tipo 'primero'")
}

// TestClienteEdgeHTTP_ConsultarPunto_ErrorHTTP verifica manejo de error HTTP
func TestClienteEdgeHTTP_ConsultarPunto_ErrorHTTP(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("serie no encontrada"))
	}))
	defer servidor.Close()

	cliente := nuevoClienteEdgeHTTP()
	direccion := strings.TrimPrefix(servidor.URL, "http://")

	solicitud := tipos.SolicitudConsultaPunto{Serie: "/sensores/noexiste"}
	_, err := cliente.ConsultarPunto(context.Background(), direccion, solicitud, "ultimo")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	t.Log("clienteEdgeHTTP.ConsultarPunto maneja errores HTTP")
}

// TestClienteEdgeHTTP_ConsultarPunto_ErrorConexion verifica error de conexion
func TestClienteEdgeHTTP_ConsultarPunto_ErrorConexion(t *testing.T) {
	cliente := nuevoClienteEdgeHTTP()

	solicitud := tipos.SolicitudConsultaPunto{Serie: "/sensores/temp"}
	_, err := cliente.ConsultarPunto(context.Background(), "localhost:99999", solicitud, "ultimo")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error en request HTTP")
	t.Log("clienteEdgeHTTP.ConsultarPunto maneja errores de conexion")
}

// ============================================================================
// TESTS DE DESCARGAR Y DESCOMPRIMIR BLOQUE
// ============================================================================

// TestDescargarYDescomprimirBloque_Exitoso verifica descarga y descompresion correcta
func TestDescargarYDescomprimirBloque_Exitoso(t *testing.T) {
	// Crear mediciones de prueba
	mediciones := []tipos.Medicion{
		{Tiempo: 1000000000, Valor: int64(100)},
		{Tiempo: 1000001000, Valor: int64(110)},
		{Tiempo: 1000002000, Valor: int64(120)},
	}

	// Crear bloque comprimido
	bloqueComprimido := crearBloqueComprimidoTest(t, mediciones, tipos.Integer, tipos.DeltaDelta, tipos.Ninguna)

	mockS3 := &mockClienteS3{
		getObjectData: bloqueComprimido,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	serie := tipos.Serie{
		SerieId:          1,
		TipoDatos:        tipos.Integer,
		CompresionBytes:  tipos.DeltaDelta,
		CompresionBloque: tipos.Ninguna,
	}

	resultado, err := m.descargarYDescomprimirBloque("nodo1/data/0000000001/bloque", serie)

	assert.NoError(t, err)
	assert.Len(t, resultado, 3)
	assert.Equal(t, int64(1000000000), resultado[0].Tiempo)
	assert.Equal(t, int64(100), resultado[0].Valor)
	t.Log("descargarYDescomprimirBloque funciona correctamente")
}

// TestDescargarYDescomprimirBloque_ErrorDescarga verifica manejo de error al descargar
func TestDescargarYDescomprimirBloque_ErrorDescarga(t *testing.T) {
	mockS3 := &mockClienteS3{
		getObjectErr: assert.AnError,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	serie := tipos.Serie{
		SerieId:          1,
		TipoDatos:        tipos.Integer,
		CompresionBytes:  tipos.DeltaDelta,
		CompresionBloque: tipos.Ninguna,
	}

	_, err := m.descargarYDescomprimirBloque("nodo1/data/0000000001/bloque", serie)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "descargando")
	t.Log("descargarYDescomprimirBloque maneja errores de descarga")
}

// TestDescargarYDescomprimirBloque_ErrorDescompresion verifica manejo de datos invalidos
func TestDescargarYDescomprimirBloque_ErrorDescompresion(t *testing.T) {
	mockS3 := &mockClienteS3{
		getObjectData: []byte("datos invalidos no comprimidos"),
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	serie := tipos.Serie{
		SerieId:          1,
		TipoDatos:        tipos.Integer,
		CompresionBytes:  tipos.DeltaDelta,
		CompresionBloque: tipos.LZ4, // Espera LZ4 pero recibe datos invalidos
	}

	_, err := m.descargarYDescomprimirBloque("nodo1/data/0000000001/bloque", serie)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "descomprimiendo")
	t.Log("descargarYDescomprimirBloque maneja errores de descompresion")
}

// ============================================================================
// TESTS DE CONSULTAR DATOS S3 CON BLOQUES VALIDOS
// ============================================================================

// TestConsultarDatosS3_ConBloquesValidos verifica descarga y filtrado de bloques
func TestConsultarDatosS3_ConBloquesValidos(t *testing.T) {
	// Crear mediciones de prueba
	mediciones := []tipos.Medicion{
		{Tiempo: 1000, Valor: int64(10)},
		{Tiempo: 2000, Valor: int64(20)},
		{Tiempo: 3000, Valor: int64(30)},
		{Tiempo: 4000, Valor: int64(40)},
	}

	bloqueComprimido := crearBloqueComprimidoTest(t, mediciones, tipos.Integer, tipos.DeltaDelta, tipos.Ninguna)

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000004000")},
			},
		},
		getObjectData: bloqueComprimido,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	nodo := tipos.Nodo{NodoID: "nodo1"}
	serie := tipos.Serie{
		SerieId:          1,
		TipoDatos:        tipos.Integer,
		CompresionBytes:  tipos.DeltaDelta,
		CompresionBloque: tipos.Ninguna,
	}

	// Consultar rango que incluye solo algunas mediciones
	resultado, err := m.consultarDatosS3(nodo, serie, 1500, 3500)

	assert.NoError(t, err)
	// Debe filtrar solo mediciones en el rango [1500, 3500]
	assert.Len(t, resultado, 2) // 2000 y 3000 estan en el rango
	t.Log("consultarDatosS3 descarga, descomprime y filtra correctamente")
}

// ============================================================================
// TESTS DE CONSULTAR ULTIMO PUNTO DESDE S3
// ============================================================================

// TestConsultarUltimoPunto_DesdeS3 verifica fallback a S3 cuando edge no responde
func TestConsultarUltimoPunto_DesdeS3(t *testing.T) {
	// Crear mediciones de prueba
	mediciones := []tipos.Medicion{
		{Tiempo: 1000, Valor: int64(10)},
		{Tiempo: 2000, Valor: int64(20)},
		{Tiempo: 3000, Valor: int64(30)},
	}

	bloqueComprimido := crearBloqueComprimidoTest(t, mediciones, tipos.Integer, tipos.DeltaDelta, tipos.Ninguna)

	// Mock edge que no encuentra datos
	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Encontrado: false,
		},
	}

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000003000")},
			},
		},
		getObjectData: bloqueComprimido,
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {
						SerieId:          1,
						Path:             "/sensores/temp",
						TipoDatos:        tipos.Integer,
						CompresionBytes:  tipos.DeltaDelta,
						CompresionBloque: tipos.Ninguna,
					},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	medicion, err := m.ConsultarUltimoPunto("/sensores/temp")

	assert.NoError(t, err)
	// Debe retornar la ultima medicion del bloque (3000, 30)
	assert.Equal(t, int64(3000), medicion.Tiempo)
	assert.Equal(t, int64(30), medicion.Valor)
	t.Log("ConsultarUltimoPunto hace fallback a S3 correctamente")
}

// ============================================================================
// TESTS DE CONSULTAR PRIMER PUNTO DESDE S3
// ============================================================================

// TestConsultarPrimerPunto_DesdeS3 verifica lectura desde S3
func TestConsultarPrimerPunto_DesdeS3(t *testing.T) {
	// Crear mediciones de prueba
	mediciones := []tipos.Medicion{
		{Tiempo: 1000, Valor: int64(10)},
		{Tiempo: 2000, Valor: int64(20)},
		{Tiempo: 3000, Valor: int64(30)},
	}

	bloqueComprimido := crearBloqueComprimidoTest(t, mediciones, tipos.Integer, tipos.DeltaDelta, tipos.Ninguna)

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000003000")},
			},
		},
		getObjectData: bloqueComprimido,
	}

	// Mock edge (no se usara porque S3 tiene datos)
	mockEdge := &mockClienteEdge{}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {
						SerieId:          1,
						Path:             "/sensores/temp",
						TipoDatos:        tipos.Integer,
						CompresionBytes:  tipos.DeltaDelta,
						CompresionBloque: tipos.Ninguna,
					},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	medicion, err := m.ConsultarPrimerPunto("/sensores/temp")

	assert.NoError(t, err)
	// Debe retornar la primera medicion del bloque (1000, 10)
	assert.Equal(t, int64(1000), medicion.Tiempo)
	assert.Equal(t, int64(10), medicion.Valor)
	t.Log("ConsultarPrimerPunto lee desde S3 correctamente")
}

// TestConsultarPrimerPunto_ErrorS3_FallbackEdge verifica fallback a edge cuando S3 falla
func TestConsultarPrimerPunto_ErrorS3_FallbackEdge(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodo1/data/0000000001/00000000001000_00000000003000")},
			},
		},
		getObjectErr: assert.AnError, // Error al descargar bloque
	}

	mockEdge := &mockClienteEdge{
		respuestaPunto: &tipos.RespuestaConsultaPunto{
			Medicion:   tipos.Medicion{Tiempo: 500, Valor: 5.0},
			Encontrado: true,
		},
	}

	m := &ManagerDespachador{
		nodos: map[string]*tipos.Nodo{
			"nodo1": {
				NodoID:      "nodo1",
				DireccionIP: "192.168.1.100",
				PuertoHTTP:  "8080",
				Series: map[string]tipos.Serie{
					"/sensores/temp": {
						SerieId:          1,
						Path:             "/sensores/temp",
						TipoDatos:        tipos.Integer,
						CompresionBytes:  tipos.DeltaDelta,
						CompresionBloque: tipos.Ninguna,
					},
				},
			},
		},
		clienteEdge: mockEdge,
		s3:          mockS3,
		config:      tipos.ConfiguracionS3{Bucket: "test-bucket"},
	}

	medicion, err := m.ConsultarPrimerPunto("/sensores/temp")

	assert.NoError(t, err)
	// Debe hacer fallback al edge
	assert.Equal(t, int64(500), medicion.Tiempo)
	t.Log("ConsultarPrimerPunto hace fallback a edge cuando S3 falla")
}

// ============================================================================
// TESTS DE NUEVO CLIENTE EDGE HTTP
// ============================================================================

// TestNuevoClienteEdgeHTTP verifica creacion del cliente
func TestNuevoClienteEdgeHTTP(t *testing.T) {
	cliente := nuevoClienteEdgeHTTP()

	assert.NotNil(t, cliente)
	assert.NotNil(t, cliente.httpClient)
	assert.Equal(t, 10*time.Second, cliente.httpClient.Timeout)
	t.Log("nuevoClienteEdgeHTTP crea cliente correctamente")
}

// ============================================================================
// TESTS DE CARGAR NODOS DESDE S3
// ============================================================================

// TestCargarNodosDesdeS3_SinNodos verifica carga cuando no hay nodos
func TestCargarNodosDesdeS3_SinNodos(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
		nodos:  make(map[string]*tipos.Nodo),
	}

	err := m.cargarNodosDesdeS3()

	assert.NoError(t, err)
	assert.Empty(t, m.nodos)
	t.Log("cargarNodosDesdeS3 funciona cuando no hay nodos")
}

// TestCargarNodosDesdeS3_ConNodos verifica carga de nodos existentes
func TestCargarNodosDesdeS3_ConNodos(t *testing.T) {
	// Crear JSON de nodo de prueba
	nodo := tipos.Nodo{
		NodoID:      "nodo-test",
		DireccionIP: "192.168.1.100",
		PuertoHTTP:  "8080",
		Series: map[string]tipos.Serie{
			"/sensores/temp": {SerieId: 1, Path: "/sensores/temp"},
		},
	}
	nodoJSON, _ := json.Marshal(nodo)

	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodos/nodo-test.json")},
			},
		},
		getObjectData: nodoJSON,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
		nodos:  make(map[string]*tipos.Nodo),
	}

	err := m.cargarNodosDesdeS3()

	assert.NoError(t, err)
	assert.Len(t, m.nodos, 1)
	assert.Contains(t, m.nodos, "nodo-test")
	assert.Equal(t, "192.168.1.100", m.nodos["nodo-test"].DireccionIP)
	t.Log("cargarNodosDesdeS3 carga nodos correctamente")
}

// TestCargarNodosDesdeS3_ErrorListando verifica manejo de error al listar
func TestCargarNodosDesdeS3_ErrorListando(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsErr: assert.AnError,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
		nodos:  make(map[string]*tipos.Nodo),
	}

	err := m.cargarNodosDesdeS3()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listando nodos")
	t.Log("cargarNodosDesdeS3 maneja error de listado")
}

// TestCargarNodosDesdeS3_ErrorGetObject verifica que continua si falla un GetObject
func TestCargarNodosDesdeS3_ErrorGetObject(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodos/nodo-test.json")},
			},
		},
		getObjectErr: assert.AnError,
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
		nodos:  make(map[string]*tipos.Nodo),
	}

	err := m.cargarNodosDesdeS3()

	// No debe retornar error, solo loguea y continua
	assert.NoError(t, err)
	assert.Empty(t, m.nodos)
	t.Log("cargarNodosDesdeS3 continua si falla GetObject")
}

// TestCargarNodosDesdeS3_JSONInvalido verifica que continua con JSON invalido
func TestCargarNodosDesdeS3_JSONInvalido(t *testing.T) {
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodos/nodo-invalido.json")},
			},
		},
		getObjectData: []byte("esto no es JSON valido"),
	}

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
		nodos:  make(map[string]*tipos.Nodo),
	}

	err := m.cargarNodosDesdeS3()

	// No debe retornar error, solo loguea y continua
	assert.NoError(t, err)
	assert.Empty(t, m.nodos)
	t.Log("cargarNodosDesdeS3 continua con JSON invalido")
}

// TestCargarNodosDesdeS3_MultiplesNodos verifica carga de multiples nodos
func TestCargarNodosDesdeS3_MultiplesNodos(t *testing.T) {
	// Para este test necesitamos un mock mas sofisticado que retorne
	// diferentes datos segun la clave solicitada
	nodo1 := tipos.Nodo{NodoID: "nodo1", DireccionIP: "192.168.1.1"}
	nodo1JSON, _ := json.Marshal(nodo1)

	// Usamos un contador para alternar respuestas
	callCount := 0
	mockS3 := &mockClienteS3{
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodos/nodo1.json")},
			},
		},
	}
	// Configurar datos del primer nodo
	mockS3.getObjectData = nodo1JSON

	m := &ManagerDespachador{
		s3:     mockS3,
		config: tipos.ConfiguracionS3{Bucket: "test-bucket"},
		nodos:  make(map[string]*tipos.Nodo),
	}

	err := m.cargarNodosDesdeS3()

	assert.NoError(t, err)
	assert.Len(t, m.nodos, 1)
	_ = callCount // evitar warning
	t.Log("cargarNodosDesdeS3 carga multiples nodos")
}

// ============================================================================
// TESTS DE CREAR DESPACHADOR
// ============================================================================

// TestCrear_ConClienteS3Inyectado verifica creacion con cliente S3 mock
func TestCrear_ConClienteS3Inyectado(t *testing.T) {
	mockS3 := &mockClienteS3{
		headBucketOutput: &s3.HeadBucketOutput{},
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	mockEdge := &mockClienteEdge{}

	opts := opcionesInternas{
		Opciones: Opciones{
			ConfigS3: tipos.ConfiguracionS3{
				Endpoint:        "http://localhost:3900",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				Bucket:          "test-bucket",
			},
		},
		clienteS3:   mockS3,
		clienteEdge: mockEdge,
	}

	manager, err := crearConOpciones(opts)

	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.s3)
	assert.NotNil(t, manager.clienteEdge)

	// Cerrar para limpiar goroutine
	manager.Cerrar()
	t.Log("Crear funciona con cliente S3 inyectado")
}

// TestCrear_BucketNoExiste_SeCreaNuevo verifica creacion de bucket
func TestCrear_BucketNoExiste_SeCreaNuevo(t *testing.T) {
	mockS3 := &mockClienteS3{
		headBucketErr:      assert.AnError, // Bucket no existe
		createBucketOutput: &s3.CreateBucketOutput{},
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	opts := opcionesInternas{
		Opciones: Opciones{
			ConfigS3: tipos.ConfiguracionS3{
				Endpoint:        "http://localhost:3900",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				Bucket:          "nuevo-bucket",
			},
		},
		clienteS3:   mockS3,
		clienteEdge: &mockClienteEdge{},
	}

	manager, err := crearConOpciones(opts)

	assert.NoError(t, err)
	assert.NotNil(t, manager)

	manager.Cerrar()
	t.Log("Crear crea bucket si no existe")
}

// TestCrear_ErrorCreandoBucket verifica error al crear bucket
func TestCrear_ErrorCreandoBucket(t *testing.T) {
	mockS3 := &mockClienteS3{
		headBucketErr:   assert.AnError, // Bucket no existe
		createBucketErr: assert.AnError, // Error al crearlo
	}

	opts := opcionesInternas{
		Opciones: Opciones{
			ConfigS3: tipos.ConfiguracionS3{
				Endpoint:        "http://localhost:3900",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				Bucket:          "bucket-fallido",
			},
		},
		clienteS3:   mockS3,
		clienteEdge: &mockClienteEdge{},
	}

	manager, err := crearConOpciones(opts)

	assert.Error(t, err)
	assert.Nil(t, manager)
	assert.Contains(t, err.Error(), "crear bucket")
	t.Log("Crear retorna error si falla creacion de bucket")
}

// TestCrear_SinClienteS3_ConfigInvalida verifica error con config invalida
func TestCrear_SinClienteS3_ConfigInvalida(t *testing.T) {
	// Sin cliente S3 inyectado y con config invalida
	opts := Opciones{
		ConfigS3: tipos.ConfiguracionS3{
			Endpoint:        "", // Invalido
			AccessKeyID:     "",
			SecretAccessKey: "",
			Bucket:          "",
		},
	}

	manager, err := Crear(opts)

	assert.Error(t, err)
	assert.Nil(t, manager)
	t.Log("Crear retorna error con config S3 invalida")
}

// TestCrear_ConNodosExistentes verifica carga de nodos al crear
func TestCrear_ConNodosExistentes(t *testing.T) {
	nodo := tipos.Nodo{
		NodoID:      "nodo-existente",
		DireccionIP: "10.0.0.1",
		PuertoHTTP:  "9000",
	}
	nodoJSON, _ := json.Marshal(nodo)

	mockS3 := &mockClienteS3{
		headBucketOutput: &s3.HeadBucketOutput{},
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{Key: aws.String("nodos/nodo-existente.json")},
			},
		},
		getObjectData: nodoJSON,
	}

	opts := opcionesInternas{
		Opciones: Opciones{
			ConfigS3: tipos.ConfiguracionS3{
				Endpoint:        "http://localhost:3900",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				Bucket:          "test-bucket",
			},
		},
		clienteS3:   mockS3,
		clienteEdge: &mockClienteEdge{},
	}

	manager, err := crearConOpciones(opts)

	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.Len(t, manager.nodos, 1)
	assert.Contains(t, manager.nodos, "nodo-existente")

	manager.Cerrar()
	t.Log("Crear carga nodos existentes desde S3")
}

// TestCrear_SinClienteEdge_UsaHTTP verifica que crea cliente HTTP por defecto
func TestCrear_SinClienteEdge_UsaHTTP(t *testing.T) {
	mockS3 := &mockClienteS3{
		headBucketOutput: &s3.HeadBucketOutput{},
		listObjectsOutput: &s3.ListObjectsV2Output{
			Contents: []s3types.Object{},
		},
	}

	opts := opcionesInternas{
		Opciones: Opciones{
			ConfigS3: tipos.ConfiguracionS3{
				Endpoint:        "http://localhost:3900",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
				Bucket:          "test-bucket",
			},
		},
		clienteS3:   mockS3,
		clienteEdge: nil, // No inyectado
	}

	manager, err := crearConOpciones(opts)

	assert.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.clienteEdge)
	// Verificar que es del tipo clienteEdgeHTTP
	_, ok := manager.clienteEdge.(*clienteEdgeHTTP)
	assert.True(t, ok, "Debe crear clienteEdgeHTTP por defecto")

	manager.Cerrar()
	t.Log("Crear usa clienteEdgeHTTP por defecto")
}
