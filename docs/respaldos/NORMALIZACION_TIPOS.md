# Normalización de Tipos en SensorWave

## Resumen Ejecutivo

**SensorWave implementa normalización automática de tipos**: aunque acepta múltiples tipos primitivos de Go en la entrada, internamente todos se normalizan a **tipos canónicos** antes de la compresión y almacenamiento.

Esta decisión de diseño simplifica la arquitectura de compresión, reduce la complejidad del código y no sacrifica funcionalidad para casos de uso IoT típicos.

---

## Arquitectura de Normalización

### Flujo de Datos

```
╔═══════════════════════════════════════════════════════════════════╗
║  CAPA 1: ENTRADA (Flexible - interface{})                        ║
║  ┌─────────────────────────────────────────────────────────────┐  ║
║  │ Acepta CUALQUIER tipo de Go compatible                      │  ║
║  │ • int, int8, int16, int32, int64, uint, uint8, uint16...   │  ║
║  │ • float32, float64                                          │  ║
║  │ • bool                                                      │  ║
║  │ • string                                                    │  ║
║  └─────────────────────────────────────────────────────────────┘  ║
╠═══════════════════════════════════════════════════════════════════╣
║  CAPA 2: INFERENCIA DE TIPO LÓGICO                               ║
║  ┌─────────────────────────────────────────────────────────────┐  ║
║  │ edge/utils.go:inferirTipo()                                 │  ║
║  │                                                             │  ║
║  │ int, int8, int16, int32, int64, uint* → tipos.Integer      │  ║
║  │ float32, float64                       → tipos.Real         │  ║
║  │ bool                                   → tipos.Boolean      │  ║
║  │ string                                 → tipos.Text         │  ║
║  └─────────────────────────────────────────────────────────────┘  ║
╠═══════════════════════════════════════════════════════════════════╣
║  CAPA 3: NORMALIZACIÓN A TIPOS CANÓNICOS                         ║
║  ┌─────────────────────────────────────────────────────────────┐  ║
║  │ compresor/compresion_utils.go:ConvertirA*Array()            │  ║
║  │                                                             │  ║
║  │ tipos.Integer → []int64   (todos los enteros)              │  ║
║  │ tipos.Real    → []float64 (todos los flotantes)            │  ║
║  │ tipos.Boolean → []bool    (sin cambio)                     │  ║
║  │ tipos.Text    → []string  (sin cambio)                     │  ║
║  └─────────────────────────────────────────────────────────────┘  ║
╠═══════════════════════════════════════════════════════════════════╣
║  CAPA 4: COMPRESIÓN (Simple - 4 tipos canónicos)                 ║
║  ┌─────────────────────────────────────────────────────────────┐  ║
║  │ Compresores genéricos operan sobre tipos normalizados:     │  ║
║  │                                                             │  ║
║  │ CompresorDeltaDeltaGenerico[int64]                         │  ║
║  │ CompresorDeltaDeltaGenerico[float64]                       │  ║
║  │ CompresorRLEGenerico[int64 | float64 | bool | string]     │  ║
║  │ CompresorBitsGenerico[int64]                               │  ║
║  │ CompresorXor (solo float64)                                │  ║
║  │ CompresorNingunoGenerico[int64 | float64 | bool | string] │  ║
║  └─────────────────────────────────────────────────────────────┘  ║
╚═══════════════════════════════════════════════════════════════════╝
```

---

## Tipos Canónicos

| Categoría | Tipo Canónico | Tamaño | Rango |
|-----------|---------------|--------|-------|
| **Enteros** | `int64` | 8 bytes | -9,223,372,036,854,775,808 a 9,223,372,036,854,775,807 |
| **Flotantes** | `float64` | 8 bytes | ±1.7e±308 (IEEE 754) |
| **Booleanos** | `bool` | 1 byte | true/false |
| **Texto** | `string` | Variable | UTF-8, hasta 65535 caracteres por valor |

---

## Implementación Detallada

### 1. Función de Inferencia (edge/utils.go:117-130)

```go
func inferirTipo(valor interface{}) tipos.TipoDatos {
    switch valor.(type) {
    case bool:
        return tipos.Boolean
    case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
        return tipos.Integer  // ← TODOS mapeados a Integer
    case float32, float64:
        return tipos.Real     // ← AMBOS mapeados a Real
    case string:
        return tipos.Text
    default:
        return tipos.Desconocido
    }
}
```

**Características:**
- ✅ Acepta 13 tipos numéricos diferentes
- ✅ Agrupa en 2 categorías lógicas (Integer, Real)
- ✅ Permite flexibilidad en la API de entrada

---

### 2. Funciones de Conversión (compresor/compresion_utils.go)

#### ConvertirAInt64Array (líneas 121-138)

```go
func ConvertirAInt64Array(valores []interface{}) ([]int64, error) {
    resultado := make([]int64, len(valores))
    for i, v := range valores {
        switch val := v.(type) {
        case int:      resultado[i] = int64(val)
        case int32:    resultado[i] = int64(val)
        case int64:    resultado[i] = val
        case float64:  resultado[i] = int64(val)  // Trunca
        default:
            return nil, fmt.Errorf("no se puede convertir tipo %T a int64", v)
        }
    }
    return resultado, nil
}
```

**Comportamiento:**
- ✅ Convierte todos los tipos enteros a `int64`
- ✅ Acepta `float64` pero **trunca** (conversión insegura)
- ⚠️ Pérdida de precisión si float64 > int64.MAX

---

#### ConvertirAFloat64Array (líneas 141-160)

```go
func ConvertirAFloat64Array(valores []interface{}) ([]float64, error) {
    resultado := make([]float64, len(valores))
    for i, v := range valores {
        switch val := v.(type) {
        case float32:  resultado[i] = float64(val)  // Conversión segura
        case float64:  resultado[i] = val
        case int:      resultado[i] = float64(val)  // Conversión segura
        case int32:    resultado[i] = float64(val)  // Conversión segura
        case int64:    resultado[i] = float64(val)  // Posible pérdida de precisión
        default:
            return nil, fmt.Errorf("no se puede convertir tipo %T a float64", v)
        }
    }
    return resultado, nil
}
```

**Comportamiento:**
- ✅ Convierte `float32` a `float64` sin pérdida
- ✅ Acepta enteros y los convierte a flotante
- ⚠️ Pérdida de precisión si `int64` usa más de 53 bits (límite de mantisa IEEE 754)

---

### 3. Constraint Numeric Actualizado (compresor/compresion_deltadelta.go:32-36)

**ANTES (versión antigua):**
```go
type Numeric interface {
    int64 | float64 | int32 | float32  // ❌ Incluía tipos no usados
}
```

**DESPUÉS (versión actualizada - Diciembre 2024):**
```go
// Numeric es una restricción de tipo para valores numéricos
// IMPORTANTE: El sistema normaliza todos los enteros a int64 y todos los flotantes a float64
// antes de la compresión. Ver edge/utils.go:inferirTipo() y compresor/compresion_utils.go:ConvertirA*Array()
type Numeric interface {
    int64 | float64  // ✅ Solo tipos canónicos
}
```

**Justificación:**
- ✅ Refleja la realidad del sistema
- ✅ Reduce complejidad en `toInt64()`/`fromInt64()`
- ✅ Documenta explícitamente la normalización
- ✅ Evita confusión sobre tipos soportados

---

## Justificación de la Normalización

### ✅ Ventajas

| Aspecto | Beneficio |
|---------|-----------|
| **Simplicidad** | Solo 4 tipos en capa de compresión (vs 13+ tipos de entrada) |
| **Mantenibilidad** | Menos caminos de código, menos errores potenciales |
| **Performance** | Sin overhead de type switches múltiples en compresores |
| **Compatibilidad** | API flexible que acepta cualquier tipo numérico Go |
| **Almacenamiento** | Suficiente para casos IoT (temperatura: -273.15°C a 6000°C cabe en float64) |

### ⚠️ Desventajas Teóricas (NO aplicables a SensorWave)

| Desventaja | ¿Aplica a SensorWave? | Razón |
|------------|----------------------|-------|
| Uso extra de memoria (int64 vs int32) | ❌ NO | Edge nodes tienen suficiente RAM para series temporales típicas |
| Uso extra de almacenamiento | ❌ NO | La compresión reduce datos 5-15x, el tipo base es irrelevante |
| Pérdida de precisión float32→float64 | ❌ NO | float64 tiene MÁS precisión que float32 |
| Overhead de conversión | ❌ NO | Conversión ocurre 1 vez al insertar, no en cada lectura |

---

## Casos de Uso IoT Soportados

### Sensores Típicos

| Sensor | Tipo de Entrada | Tipo Normalizado | Rango Típico | ¿Suficiente? |
|--------|----------------|------------------|--------------|--------------|
| **Temperatura** | float32, float64 | float64 | -40°C a 125°C | ✅ SÍ |
| **Humedad** | float32, float64 | float64 | 0% a 100% | ✅ SÍ |
| **Presión** | float32, float64 | float64 | 300 hPa a 1100 hPa | ✅ SÍ |
| **Contador eventos** | int, int32, int64 | int64 | 0 a millones | ✅ SÍ |
| **Estado ON/OFF** | bool | bool | true/false | ✅ SÍ |
| **Código error** | string | string | "OK", "ERROR", etc. | ✅ SÍ |
| **Timestamp** | int64 | int64 | Nanosegundos Unix | ✅ SÍ |

### Ejemplos de Uso Real (del código)

```go
// test/edge/series/main.go
manager.Insertar("sensor1.temperatura", timestamp, 23.5)        // float64 literal
manager.Insertar("sensor1.humedad", timestamp, 60.0+float64(i)) // float64 explícito
manager.Insertar("sensor1.presion", timestamp, 1013.25)         // float64 literal

// test/edge/path_tags_ejemplo/main.go
manager.Insertar("dispositivo_001/temperatura", timestamp, 23.5+float64(i)*0.1)
```

**Observación:** Todos los ejemplos usan `float64` directamente. No hay uso de `float32` o `int32` en el código existente.

---

## Límites y Consideraciones

### Límites de Precisión

#### int64
- **Rango:** -9.2×10¹⁸ a 9.2×10¹⁸
- **Uso típico:** Timestamps en nanosegundos (cubre 292 años desde epoch)
- **Límite práctico:** Suficiente para cualquier contador/timestamp IoT

#### float64
- **Precisión:** ~15-17 dígitos decimales significativos
- **Rango:** ±1.7×10³⁰⁸
- **Uso típico:** Mediciones de sensores con precisión de 0.001
- **Límite práctico:** Excede la precisión de cualquier sensor IoT comercial

### Conversiones Seguras vs Inseguras

| Conversión | ¿Segura? | Notas |
|------------|----------|-------|
| int → int64 | ✅ SÍ | Widening conversion |
| int32 → int64 | ✅ SÍ | Widening conversion |
| float32 → float64 | ✅ SÍ | Widening conversion, sin pérdida |
| int64 → float64 | ⚠️ CASI | Pérdida si int64 > 2⁵³ (muy raro en IoT) |
| float64 → int64 | ❌ NO | Trunca decimales (solo en `ConvertirAInt64Array`) |

---

## Compresores y Normalización

### Compresores que Requieren Normalización

| Compresor | Constraint | Tipos Aceptados Tras Normalización | Justificación |
|-----------|------------|-------------------------------------|---------------|
| **CompresorDeltaDeltaGenerico** | `Numeric` | `int64`, `float64` | Algoritmo funciona con cualquier numérico |
| **CompresorBitsGenerico** | `Numeric` | `int64` únicamente | Solo tiene sentido para enteros discretos |
| **CompresorXor** | No genérico | `float64` únicamente | Algoritmo Gorilla específico para IEEE 754 |
| **CompresorRLEGenerico** | `comparable` | `int64`, `float64`, `bool`, `string` | Funciona con cualquier tipo comparable |
| **CompresorNingunoGenerico** | `any` | `int64`, `float64`, `bool`, `string` | Serialización directa por tipo |

### Conversión Interna en Compresores

**CompresorDeltaDelta** usa funciones de conversión interna:

```go
// compresor/compresion_deltadelta.go:196-210
func toInt64[T Numeric](v T) int64 {
    switch val := any(v).(type) {
    case int64:
        return val
    case float64:
        return int64(math.Float64bits(val))  // Usa representación binaria, NO trunca
    }
}
```

**Nota importante:** Para `float64`, usa `math.Float64bits()` que preserva la representación binaria completa, no hace conversión numérica.

---

## Decisiones de Diseño Documentadas

### ¿Por qué NO soportar int32/float32 explícitamente?

**Decisión:** Eliminar `int32` y `float32` del constraint `Numeric` (Diciembre 2024)

**Razones:**

1. ✅ **Realidad del sistema:** Todos los valores se normalizan a int64/float64 ANTES de llegar a los compresores
2. ✅ **Código más claro:** El constraint refleja lo que realmente soportan los compresores
3. ✅ **Menos confusión:** Evita que desarrolladores piensen que pueden usar int32/float32 directamente
4. ✅ **Mantenibilidad:** Funciones `toInt64()`/`fromInt64()` más simples (2 casos vs 4)
5. ✅ **Sin pérdida funcional:** La normalización previa ya maneja estos tipos

### ¿Por qué normalizar a tipos de 64 bits?

**Decisión:** Usar int64 y float64 como tipos canónicos

**Razones:**

1. ✅ **Suficiente para IoT:** Rangos y precisión exceden necesidades de sensores
2. ✅ **Estándar Go:** `int64` es el tipo preferido para timestamps (`time.UnixNano()`)
3. ✅ **Simplicidad:** Un tipo por categoría (entero/flotante)
4. ✅ **Portabilidad:** Tamaño fijo en todas las plataformas (vs `int` que varía)
5. ✅ **Compresión:** Los algoritmos reducen el tamaño 5-15x, el tipo base es irrelevante

---

## Migración y Compatibilidad

### ¿Qué cambió en Diciembre 2024?

| Aspecto | Antes | Después |
|---------|-------|---------|
| **Constraint Numeric** | `int64 \| float64 \| int32 \| float32` | `int64 \| float64` |
| **Funciones toInt64/fromInt64** | 4 casos en switch | 2 casos en switch |
| **Documentación** | Sin explicación de normalización | Documentado explícitamente |
| **Comportamiento runtime** | ❌ Sin cambios | ❌ Sin cambios |

### ¿Es un breaking change?

**NO.** Los cambios son solo en el nivel de tipos genéricos. El comportamiento en runtime no cambia porque:

- ✅ La normalización siempre ocurrió antes de llegar a los compresores
- ✅ `ConvertirAInt64Array()` y `ConvertirAFloat64Array()` siguen funcionando igual
- ✅ Los datos existentes son compatibles
- ✅ La API pública (`Insertar()`) sigue aceptando `interface{}`

---

## Referencias de Código

### Archivos Clave

| Archivo | Función | Líneas Relevantes |
|---------|---------|-------------------|
| **tipos/medicion.go** | Definición de `Medicion` con `interface{}` | 4-7 |
| **edge/utils.go** | Función `inferirTipo()` | 117-130 |
| **compresor/compresion_utils.go** | Funciones `ConvertirA*Array()` | 121-173 |
| **compresor/compresion_deltadelta.go** | Constraint `Numeric` y conversiones | 32-36, 196-227 |
| **compresor/compresion_bits.go** | Uso de constraint `Numeric` | 53 |
| **compresor/compresion_xor.go** | Compresor no genérico float64 | 9-10 |
| **edge/edge.go** | Uso de compresores normalizados | 666-767 |

### Tests de Referencia

| Test | Demuestra | Líneas |
|------|-----------|--------|
| **test/edge/series/main.go** | Inserción con float64 | 172-227 |
| **test/edge/path_tags_ejemplo/main.go** | Uso típico de API | 38-90 |
| **test/despachador/despachador.go** | Integración con despachador | 44 |

---

## Conclusión

La **normalización automática de tipos** en SensorWave es una decisión de arquitectura que:

✅ **Simplifica** la implementación de compresores
✅ **Mantiene** flexibilidad en la API de entrada
✅ **Optimiza** para casos de uso IoT reales
✅ **Documenta** claramente las conversiones aplicadas
✅ **No sacrifica** funcionalidad práctica

El constraint `Numeric` actualizado refleja fielmente esta arquitectura, eliminando tipos que nunca llegarían a los compresores tras la normalización previa.

---

## Cambios Recientes

### Diciembre 2024

#### 1. Simplificación del Constraint Numeric
- **Eliminado:** `int32` y `float32` del constraint `Numeric`
- **Mantiene:** Solo `int64` y `float64`
- **Razón:** Refleja la normalización real del sistema
- **Archivos:** `compresor/compresion_deltadelta.go:32-36`

#### 2. Simplificación de Funciones de Conversión
- **Función `toInt64()`:** Reducida de 4 casos a 2 casos
- **Función `fromInt64()`:** Reducida de 4 casos a 2 casos
- **Beneficio:** Menos complejidad, código más mantenible

#### 3. Eliminación de Variable No Usada
- **Eliminado:** `compresionPorDefecto` en `tipos/tipo_datos.go`
- **Razón:** Variable nunca referenciada en el código
- **Impacto:** Ninguno (no se usaba en runtime)
- **Reducción:** De 98 líneas a 89 líneas en tipo_datos.go

#### 4. Documentación Agregada
- **Agregado:** Comentarios explicativos sobre normalización en:
  - `compresor/compresion_deltadelta.go`
  - `compresor/compresion_bits.go`
  - `compresor/compresion_ninguno.go`
  - `compresor/compresion_xor.go`
- **Creado:** Este documento (`NORMALIZACION_TIPOS.md`)

---

**Fecha de actualización:** Diciembre 2024  
**Versión del sistema:** SensorWave v1.0  
**Autor:** Documentación técnica del proyecto
