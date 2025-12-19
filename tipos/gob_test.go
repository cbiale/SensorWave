package tipos

import (
	"testing"
	"time"
)

// ==================== Tests de SerializarGob ====================

// TestSerializarDeserializarGob_Medicion verifica serialización de Medicion
func TestSerializarDeserializarGob_Medicion(t *testing.T) {
	original := Medicion{
		Tiempo: time.Now().UnixNano(),
		Valor:  float64(25.5),
	}

	// Serializar
	data, err := SerializarGob(original)
	if err != nil {
		t.Fatalf("Error al serializar Medicion: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Datos serializados vacíos")
	}

	// Deserializar
	var deserializada Medicion
	err = DeserializarGob(data, &deserializada)
	if err != nil {
		t.Fatalf("Error al deserializar Medicion: %v", err)
	}

	// Verificar
	if deserializada.Tiempo != original.Tiempo {
		t.Errorf("Tiempo incorrecto: esperado %d, obtenido %d", original.Tiempo, deserializada.Tiempo)
	}

	valorOriginal := original.Valor.(float64)
	valorDeserializado := deserializada.Valor.(float64)
	if valorDeserializado != valorOriginal {
		t.Errorf("Valor incorrecto: esperado %f, obtenido %f", valorOriginal, valorDeserializado)
	}

	t.Logf("✓ Medicion serializada/deserializada correctamente: tiempo=%d, valor=%v", deserializada.Tiempo, deserializada.Valor)
}

// TestSerializarDeserializarGob_MedicionTodosTipos verifica serialización con diferentes tipos de valor
func TestSerializarDeserializarGob_MedicionTodosTipos(t *testing.T) {
	testCases := []struct {
		nombre string
		valor  interface{}
	}{
		{"Boolean true", true},
		{"Boolean false", false},
		{"Integer", int64(12345)},
		{"Real", float64(3.14159)},
		{"Text", "sensor_value"},
	}

	for _, tc := range testCases {
		t.Run(tc.nombre, func(t *testing.T) {
			original := Medicion{
				Tiempo: time.Now().UnixNano(),
				Valor:  tc.valor,
			}

			data, err := SerializarGob(original)
			if err != nil {
				t.Fatalf("Error al serializar: %v", err)
			}

			var deserializada Medicion
			err = DeserializarGob(data, &deserializada)
			if err != nil {
				t.Fatalf("Error al deserializar: %v", err)
			}

			if deserializada.Valor != tc.valor {
				t.Errorf("Valor incorrecto: esperado %v (%T), obtenido %v (%T)",
					tc.valor, tc.valor, deserializada.Valor, deserializada.Valor)
			}

			t.Logf("✓ Tipo %s serializado correctamente", tc.nombre)
		})
	}
}

// TestSerializarDeserializarGob_Serie verifica serialización de Serie
func TestSerializarDeserializarGob_Serie(t *testing.T) {
	original := Serie{
		SerieId:              123,
		Path:                 "dispositivo/temperatura",
		TipoDatos:            Real,
		CompresionBloque:     LZ4,
		CompresionBytes:      DeltaDelta,
		TamañoBloque:         1000,
		TiempoAlmacenamiento: 7 * 24 * time.Hour.Nanoseconds(),
		Tags: map[string]string{
			"ubicacion": "sala1",
			"tipo":      "DHT22",
		},
	}

	// Serializar
	data, err := SerializarGob(original)
	if err != nil {
		t.Fatalf("Error al serializar Serie: %v", err)
	}

	// Deserializar
	var deserializada Serie
	err = DeserializarGob(data, &deserializada)
	if err != nil {
		t.Fatalf("Error al deserializar Serie: %v", err)
	}

	// Verificar campos
	if deserializada.SerieId != original.SerieId {
		t.Errorf("SerieId incorrecto: %d != %d", deserializada.SerieId, original.SerieId)
	}
	if deserializada.Path != original.Path {
		t.Errorf("Path incorrecto: %s != %s", deserializada.Path, original.Path)
	}
	if deserializada.TipoDatos != original.TipoDatos {
		t.Errorf("TipoDatos incorrecto: %v != %v", deserializada.TipoDatos, original.TipoDatos)
	}
	if deserializada.TamañoBloque != original.TamañoBloque {
		t.Errorf("TamañoBloque incorrecto: %d != %d", deserializada.TamañoBloque, original.TamañoBloque)
	}
	if deserializada.Tags["ubicacion"] != "sala1" {
		t.Errorf("Tag 'ubicacion' incorrecto: %s", deserializada.Tags["ubicacion"])
	}

	t.Log("✓ Serie serializada/deserializada correctamente")
}

// TestSerializarDeserializarGob_SolicitudConsultaRango verifica serialización de solicitud
func TestSerializarDeserializarGob_SolicitudConsultaRango(t *testing.T) {
	original := SolicitudConsultaRango{
		Serie:        "sensor/temperatura",
		TiempoInicio: time.Now().Add(-1 * time.Hour).UnixNano(),
		TiempoFin:    time.Now().UnixNano(),
	}

	data, err := SerializarGob(original)
	if err != nil {
		t.Fatalf("Error al serializar SolicitudConsultaRango: %v", err)
	}

	var deserializada SolicitudConsultaRango
	err = DeserializarGob(data, &deserializada)
	if err != nil {
		t.Fatalf("Error al deserializar SolicitudConsultaRango: %v", err)
	}

	if deserializada.Serie != original.Serie {
		t.Errorf("Serie incorrecta: %s != %s", deserializada.Serie, original.Serie)
	}
	if deserializada.TiempoInicio != original.TiempoInicio {
		t.Errorf("TiempoInicio incorrecto")
	}
	if deserializada.TiempoFin != original.TiempoFin {
		t.Errorf("TiempoFin incorrecto")
	}

	t.Log("✓ SolicitudConsultaRango serializada/deserializada correctamente")
}

// TestSerializarDeserializarGob_SolicitudConsultaPunto verifica serialización de consulta punto
func TestSerializarDeserializarGob_SolicitudConsultaPunto(t *testing.T) {
	original := SolicitudConsultaPunto{
		Serie: "sensor/humedad",
	}

	data, err := SerializarGob(original)
	if err != nil {
		t.Fatalf("Error al serializar SolicitudConsultaPunto: %v", err)
	}

	var deserializada SolicitudConsultaPunto
	err = DeserializarGob(data, &deserializada)
	if err != nil {
		t.Fatalf("Error al deserializar SolicitudConsultaPunto: %v", err)
	}

	if deserializada.Serie != original.Serie {
		t.Errorf("Serie incorrecta")
	}

	t.Log("✓ SolicitudConsultaPunto serializada/deserializada correctamente")
}

// TestSerializarDeserializarGob_RespuestaConsultaRango verifica respuesta con mediciones
func TestSerializarDeserializarGob_RespuestaConsultaRango(t *testing.T) {
	original := RespuestaConsultaRango{
		Mediciones: []Medicion{
			{Tiempo: time.Now().UnixNano(), Valor: float64(25.0)},
			{Tiempo: time.Now().Add(time.Second).UnixNano(), Valor: float64(26.0)},
			{Tiempo: time.Now().Add(2 * time.Second).UnixNano(), Valor: float64(27.0)},
		},
		Error: "",
	}

	data, err := SerializarGob(original)
	if err != nil {
		t.Fatalf("Error al serializar RespuestaConsultaRango: %v", err)
	}

	var deserializada RespuestaConsultaRango
	err = DeserializarGob(data, &deserializada)
	if err != nil {
		t.Fatalf("Error al deserializar RespuestaConsultaRango: %v", err)
	}

	if len(deserializada.Mediciones) != 3 {
		t.Errorf("Cantidad de mediciones incorrecta: esperadas 3, obtenidas %d", len(deserializada.Mediciones))
	}

	t.Logf("✓ RespuestaConsultaRango con %d mediciones serializada correctamente", len(deserializada.Mediciones))
}

// TestDeserializarGob_DatosInvalidos verifica error con datos corruptos
func TestDeserializarGob_DatosInvalidos(t *testing.T) {
	datosInvalidos := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}

	var medicion Medicion
	err := DeserializarGob(datosInvalidos, &medicion)
	if err == nil {
		t.Error("Se esperaba error al deserializar datos inválidos")
	} else {
		t.Logf("✓ Error esperado con datos inválidos: %v", err)
	}
}

// TestDeserializarGob_DatosVacios verifica error con datos vacíos
func TestDeserializarGob_DatosVacios(t *testing.T) {
	datosVacios := []byte{}

	var medicion Medicion
	err := DeserializarGob(datosVacios, &medicion)
	if err == nil {
		t.Error("Se esperaba error al deserializar datos vacíos")
	} else {
		t.Logf("✓ Error esperado con datos vacíos: %v", err)
	}
}

// TestSerializarGob_SliceMediciones verifica serialización de slice
func TestSerializarGob_SliceMediciones(t *testing.T) {
	original := []Medicion{
		{Tiempo: 1000000000, Valor: int64(100)},
		{Tiempo: 2000000000, Valor: int64(200)},
		{Tiempo: 3000000000, Valor: int64(300)},
	}

	data, err := SerializarGob(original)
	if err != nil {
		t.Fatalf("Error al serializar slice: %v", err)
	}

	var deserializado []Medicion
	err = DeserializarGob(data, &deserializado)
	if err != nil {
		t.Fatalf("Error al deserializar slice: %v", err)
	}

	if len(deserializado) != len(original) {
		t.Errorf("Longitud incorrecta: %d != %d", len(deserializado), len(original))
	}

	t.Logf("✓ Slice de %d mediciones serializado correctamente", len(deserializado))
}

// TestSerializarGob_ErrorTipoNoSerializable verifica error con tipo no serializable
func TestSerializarGob_ErrorTipoNoSerializable(t *testing.T) {
	// Los canales no se pueden serializar con gob
	ch := make(chan int)

	_, err := SerializarGob(ch)
	if err == nil {
		t.Error("Se esperaba error al serializar un channel")
	} else {
		t.Logf("✓ Error esperado con channel: %v", err)
	}
}

// TestSerializarGob_ErrorFuncion verifica error con función
func TestSerializarGob_ErrorFuncion(t *testing.T) {
	// Las funciones no se pueden serializar con gob
	fn := func() {}

	_, err := SerializarGob(fn)
	if err == nil {
		t.Error("Se esperaba error al serializar una función")
	} else {
		t.Logf("✓ Error esperado con función: %v", err)
	}
}
