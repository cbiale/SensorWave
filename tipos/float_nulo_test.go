package tipos

import (
	"encoding/json"
	"math"
	"testing"
)

// TestFloatNulo_MarshalJSON_ValorNormal verifica serialización de valores normales
func TestFloatNulo_MarshalJSON_ValorNormal(t *testing.T) {
	casos := []struct {
		nombre   string
		valor    FloatNulo
		esperado string
	}{
		{"cero", FloatNulo(0), "0"},
		{"positivo", FloatNulo(25.5), "25.5"},
		{"negativo", FloatNulo(-10.3), "-10.3"},
		{"entero", FloatNulo(100), "100"},
		{"muy pequeño", FloatNulo(0.001), "0.001"},
		{"muy grande", FloatNulo(1e10), "10000000000"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			data, err := json.Marshal(c.valor)
			if err != nil {
				t.Fatalf("Error serializando: %v", err)
			}
			if string(data) != c.esperado {
				t.Errorf("Esperado %s, obtenido %s", c.esperado, string(data))
			}
		})
	}
}

// TestFloatNulo_MarshalJSON_NaN verifica que NaN se serializa como null
func TestFloatNulo_MarshalJSON_NaN(t *testing.T) {
	valor := FloatNulo(math.NaN())
	data, err := json.Marshal(valor)
	if err != nil {
		t.Fatalf("Error serializando NaN: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("Esperado 'null', obtenido '%s'", string(data))
	}
}

// TestFloatNulo_UnmarshalJSON_ValorNormal verifica deserialización de valores normales
func TestFloatNulo_UnmarshalJSON_ValorNormal(t *testing.T) {
	casos := []struct {
		nombre   string
		json     string
		esperado float64
	}{
		{"cero", "0", 0},
		{"positivo", "25.5", 25.5},
		{"negativo", "-10.3", -10.3},
		{"entero", "100", 100},
		{"muy pequeño", "0.001", 0.001},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var valor FloatNulo
			if err := json.Unmarshal([]byte(c.json), &valor); err != nil {
				t.Fatalf("Error deserializando: %v", err)
			}
			if float64(valor) != c.esperado {
				t.Errorf("Esperado %f, obtenido %f", c.esperado, float64(valor))
			}
		})
	}
}

// TestFloatNulo_UnmarshalJSON_Null verifica que null se deserializa como NaN
func TestFloatNulo_UnmarshalJSON_Null(t *testing.T) {
	var valor FloatNulo
	if err := json.Unmarshal([]byte("null"), &valor); err != nil {
		t.Fatalf("Error deserializando null: %v", err)
	}
	if !math.IsNaN(float64(valor)) {
		t.Errorf("Esperado NaN, obtenido %f", float64(valor))
	}
}

// TestFloatNulo_RoundTrip verifica serialización y deserialización completa
func TestFloatNulo_RoundTrip(t *testing.T) {
	casos := []struct {
		nombre string
		valor  FloatNulo
		esNaN  bool
	}{
		{"valor normal", FloatNulo(42.5), false},
		{"cero", FloatNulo(0), false},
		{"NaN", FloatNulo(math.NaN()), true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			// Serializar
			data, err := json.Marshal(c.valor)
			if err != nil {
				t.Fatalf("Error serializando: %v", err)
			}

			// Deserializar
			var resultado FloatNulo
			if err := json.Unmarshal(data, &resultado); err != nil {
				t.Fatalf("Error deserializando: %v", err)
			}

			// Verificar
			if c.esNaN {
				if !math.IsNaN(float64(resultado)) {
					t.Errorf("Esperado NaN, obtenido %f", float64(resultado))
				}
			} else {
				if float64(resultado) != float64(c.valor) {
					t.Errorf("Esperado %f, obtenido %f", float64(c.valor), float64(resultado))
				}
			}
		})
	}
}

// TestFloatNulo_Slice verifica serialización de slices
func TestFloatNulo_Slice(t *testing.T) {
	original := []FloatNulo{
		FloatNulo(1.0),
		FloatNulo(math.NaN()),
		FloatNulo(3.0),
		FloatNulo(math.NaN()),
		FloatNulo(5.0),
	}

	// Serializar
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Error serializando slice: %v", err)
	}

	esperado := "[1,null,3,null,5]"
	if string(data) != esperado {
		t.Errorf("Esperado %s, obtenido %s", esperado, string(data))
	}

	// Deserializar
	var resultado []FloatNulo
	if err := json.Unmarshal(data, &resultado); err != nil {
		t.Fatalf("Error deserializando slice: %v", err)
	}

	if len(resultado) != len(original) {
		t.Fatalf("Longitud diferente: esperado %d, obtenido %d", len(original), len(resultado))
	}

	for i := range original {
		origNaN := math.IsNaN(float64(original[i]))
		resNaN := math.IsNaN(float64(resultado[i]))

		if origNaN != resNaN {
			t.Errorf("Índice %d: NaN mismatch", i)
		} else if !origNaN && float64(original[i]) != float64(resultado[i]) {
			t.Errorf("Índice %d: esperado %f, obtenido %f", i, float64(original[i]), float64(resultado[i]))
		}
	}
}

// TestFloatNulo_Matriz verifica serialización de matrices [][]FloatNulo
func TestFloatNulo_Matriz(t *testing.T) {
	original := [][]FloatNulo{
		{FloatNulo(1.0), FloatNulo(2.0), FloatNulo(math.NaN())},
		{FloatNulo(math.NaN()), FloatNulo(4.0), FloatNulo(5.0)},
	}

	// Serializar
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Error serializando matriz: %v", err)
	}

	esperado := "[[1,2,null],[null,4,5]]"
	if string(data) != esperado {
		t.Errorf("Esperado %s, obtenido %s", esperado, string(data))
	}

	// Deserializar
	var resultado [][]FloatNulo
	if err := json.Unmarshal(data, &resultado); err != nil {
		t.Fatalf("Error deserializando matriz: %v", err)
	}

	if len(resultado) != len(original) {
		t.Fatalf("Filas diferentes: esperado %d, obtenido %d", len(original), len(resultado))
	}

	for i := range original {
		if len(resultado[i]) != len(original[i]) {
			t.Fatalf("Fila %d: columnas diferentes: esperado %d, obtenido %d", i, len(original[i]), len(resultado[i]))
		}
		for j := range original[i] {
			origNaN := math.IsNaN(float64(original[i][j]))
			resNaN := math.IsNaN(float64(resultado[i][j]))

			if origNaN != resNaN {
				t.Errorf("Posición [%d][%d]: NaN mismatch", i, j)
			} else if !origNaN && float64(original[i][j]) != float64(resultado[i][j]) {
				t.Errorf("Posición [%d][%d]: esperado %f, obtenido %f", i, j, float64(original[i][j]), float64(resultado[i][j]))
			}
		}
	}
}

// TestFloatNulo_Matriz3D verifica serialización de matrices [][][]FloatNulo
func TestFloatNulo_Matriz3D(t *testing.T) {
	original := [][][]FloatNulo{
		{
			{FloatNulo(1.0), FloatNulo(math.NaN())},
			{FloatNulo(3.0), FloatNulo(4.0)},
		},
		{
			{FloatNulo(math.NaN()), FloatNulo(6.0)},
			{FloatNulo(7.0), FloatNulo(math.NaN())},
		},
	}

	// Serializar
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Error serializando matriz 3D: %v", err)
	}

	esperado := "[[[1,null],[3,4]],[[null,6],[7,null]]]"
	if string(data) != esperado {
		t.Errorf("Esperado %s, obtenido %s", esperado, string(data))
	}

	// Deserializar
	var resultado [][][]FloatNulo
	if err := json.Unmarshal(data, &resultado); err != nil {
		t.Fatalf("Error deserializando matriz 3D: %v", err)
	}

	// Verificar dimensiones y valores
	if len(resultado) != len(original) {
		t.Fatalf("Dimensión 0 diferente: esperado %d, obtenido %d", len(original), len(resultado))
	}

	for i := range original {
		for j := range original[i] {
			for k := range original[i][j] {
				origNaN := math.IsNaN(float64(original[i][j][k]))
				resNaN := math.IsNaN(float64(resultado[i][j][k]))

				if origNaN != resNaN {
					t.Errorf("Posición [%d][%d][%d]: NaN mismatch", i, j, k)
				} else if !origNaN && float64(original[i][j][k]) != float64(resultado[i][j][k]) {
					t.Errorf("Posición [%d][%d][%d]: esperado %f, obtenido %f", i, j, k, float64(original[i][j][k]), float64(resultado[i][j][k]))
				}
			}
		}
	}
}

// TestFloatNulo_EsNulo verifica el método EsNulo
func TestFloatNulo_EsNulo(t *testing.T) {
	if FloatNulo(25.5).EsNulo() {
		t.Error("25.5 no debería ser nulo")
	}
	if FloatNulo(0).EsNulo() {
		t.Error("0 no debería ser nulo")
	}
	if !FloatNulo(math.NaN()).EsNulo() {
		t.Error("NaN debería ser nulo")
	}
	if !FloatNuloVacio().EsNulo() {
		t.Error("FloatNuloVacio() debería ser nulo")
	}
}

// TestFloatNulo_Valor verifica el método Valor
func TestFloatNulo_Valor(t *testing.T) {
	f := FloatNulo(42.5)
	if f.Valor() != 42.5 {
		t.Errorf("Esperado 42.5, obtenido %f", f.Valor())
	}
}

// TestFloatNulo_Constructores verifica los constructores
func TestFloatNulo_Constructores(t *testing.T) {
	// NuevoFloatNulo
	f := NuevoFloatNulo(25.5)
	if f.Valor() != 25.5 {
		t.Errorf("NuevoFloatNulo: esperado 25.5, obtenido %f", f.Valor())
	}

	// FloatNuloVacio
	vacio := FloatNuloVacio()
	if !vacio.EsNulo() {
		t.Error("FloatNuloVacio debería retornar un valor nulo")
	}
}

// TestFloatNulo_EnStruct verifica uso dentro de un struct
func TestFloatNulo_EnStruct(t *testing.T) {
	type Resultado struct {
		Series  []string      `json:"series"`
		Valores [][]FloatNulo `json:"valores"`
	}

	original := Resultado{
		Series: []string{"temp", "humedad"},
		Valores: [][]FloatNulo{
			{FloatNulo(25.5), FloatNulo(math.NaN())},
		},
	}

	// Serializar
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Error serializando struct: %v", err)
	}

	// Verificar JSON
	esperado := `{"series":["temp","humedad"],"valores":[[25.5,null]]}`
	if string(data) != esperado {
		t.Errorf("Esperado %s, obtenido %s", esperado, string(data))
	}

	// Deserializar
	var resultado Resultado
	if err := json.Unmarshal(data, &resultado); err != nil {
		t.Fatalf("Error deserializando struct: %v", err)
	}

	// Verificar
	if len(resultado.Series) != 2 {
		t.Errorf("Series: esperado 2, obtenido %d", len(resultado.Series))
	}
	if !math.IsNaN(float64(resultado.Valores[0][1])) {
		t.Error("Valores[0][1] debería ser NaN")
	}
	if float64(resultado.Valores[0][0]) != 25.5 {
		t.Errorf("Valores[0][0]: esperado 25.5, obtenido %f", float64(resultado.Valores[0][0]))
	}
}
