package tipos

import (
	"encoding/json"
	"math"
)

// FloatNulo es un float64 que serializa math.NaN() como null en JSON.
// Permite representar valores faltantes de forma compatible con JSON.
//
// Uso:
//   - En Go: usar math.IsNaN(float64(valor)) para verificar si es nulo
//   - En JSON: se serializa como null cuando el valor es NaN
//   - Desde JSON: null se deserializa como math.NaN()
//
// Este tipo existe para resolver la incompatibilidad entre JSON (que no soporta NaN)
// y la representación interna de valores faltantes en consultas de agregación.
type FloatNulo float64

// MarshalJSON serializa el valor a JSON.
// Si el valor es NaN, serializa como null.
func (f FloatNulo) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(f)) {
		return []byte("null"), nil
	}
	return json.Marshal(float64(f))
}

// UnmarshalJSON deserializa el valor desde JSON.
// Si el valor es null, deserializa como NaN.
func (f *FloatNulo) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = FloatNulo(math.NaN())
		return nil
	}
	var val float64
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	*f = FloatNulo(val)
	return nil
}

// EsNulo retorna true si el valor representa un valor faltante (NaN).
func (f FloatNulo) EsNulo() bool {
	return math.IsNaN(float64(f))
}

// Valor retorna el valor como float64.
func (f FloatNulo) Valor() float64 {
	return float64(f)
}

// NuevoFloatNulo crea un FloatNulo a partir de un float64.
func NuevoFloatNulo(v float64) FloatNulo {
	return FloatNulo(v)
}

// FloatNuloVacio retorna un FloatNulo que representa un valor faltante (NaN).
func FloatNuloVacio() FloatNulo {
	return FloatNulo(math.NaN())
}
