# Sistema de Buffering en SensorWave

## Descripción General

SensorWave utiliza un sistema de **buffering asíncrono por serie** para optimizar la ingesta de datos desde sensores IoT. Cada serie temporal tiene su propio canal de buffer independiente, lo que permite procesar datos concurrentemente sin bloqueos entre diferentes series.

## Arquitectura

```
┌─────────────────────────────────────────────────────────────┐
│                  Sensor/Aplicación                          │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
            ManagerEdge.Insertar(path, timestamp, valor)
                       │
                       ▼
         ┌─────────────────────────────┐
         │  Validación de tipo         │
         │  y existencia de serie      │
         └─────────────┬───────────────┘
                       │
                       ▼
         ┌─────────────────────────────────────────┐
         │  Canal buffered (configurable)          │
         │  Tamaño: TamañoBuffer (default: 1000)   │
         │  Timeout: TimeoutBuffer (default: 100ms)│
         └─────────────┬───────────────────────────┘
                       │
                       ▼
         ┌─────────────────────────────┐
         │  Goroutine: manejarBuffer() │
         │  (una por serie)            │
         └─────────────┬───────────────┘
                       │
                       ▼
         ┌─────────────────────────────┐
         │  Buffer en memoria          │
         │  (TamañoBloque mediciones)  │
         └─────────────┬───────────────┘
                       │
                       ▼ (cuando está lleno)
         ┌─────────────────────────────┐
         │  Compresión de dos niveles  │
         │  1. Bytes (DeltaDelta/Xor)  │
         │  2. Bloque (LZ4/ZSTD/etc)   │
         └─────────────┬───────────────┘
                       │
                       ▼
         ┌─────────────────────────────┐
         │  Persistencia en PebbleDB   │
         └─────────────────────────────┘
```

## Configuración

### Parámetros de Buffer

Los parámetros del sistema de buffering se configuran al crear una instancia de `ManagerEdge`:

```go
import (
    "github.com/cbiale/sensorwave/edge"
    "github.com/cbiale/sensorwave/tipos"
)

// Configuración con valores por defecto
manager, err := edge.Crear(tipos.OpcionesEdge{
    NombreDB:   "mi_base_datos.db",
    PuertoHTTP: "4240",
    // TamañoBuffer y TimeoutBuffer usan defaults
})

// Configuración personalizada
manager, err := edge.Crear(tipos.OpcionesEdge{
    NombreDB:      "mi_base_datos.db",
    PuertoHTTP:    "4240",
    TamañoBuffer:  2000,                    // 2000 mediciones por canal
    TimeoutBuffer: 200 * 1000 * 1000,       // 200ms en nanosegundos
})
```

### Tabla de Parámetros

| Parámetro | Tipo | Default | Descripción |
|-----------|------|---------|-------------|
| `TamañoBuffer` | `int` | `1000` | Capacidad del canal de buffer **por serie** |
| `TimeoutBuffer` | `int64` | `100000000` (100ms) | Timeout en nanosegundos para inserción |

## Valores por Defecto

### TamañoBuffer: 1000

El buffer de **1000 mediciones por serie** está diseñado para:

- ✅ **Manejar burst de datos**: Sensores que envían lotes de lecturas acumuladas
- ✅ **Absorber variabilidad**: Compensar diferencias entre velocidad de ingesta y compresión
- ✅ **Memoria razonable**: ~24 KB por serie (1000 × 24 bytes aprox.)

**Ejemplo de carga sostenible:**
- 10 series activas × 1000 mediciones = 10,000 mediciones en buffers
- Memoria total en buffers: ~240 KB

### TimeoutBuffer: 100ms

El timeout de **100 milisegundos** permite:

- ✅ **Detección rápida de saturación**: No bloquear indefinidamente
- ✅ **Latencia aceptable**: Típica en sistemas IoT
- ✅ **Balance**: Tiempo suficiente para que goroutines procesen datos

## Comportamiento del Sistema

### Escenario 1: Operación Normal

```
Sensor → Insertar() → [Canal con espacio] → Retorna inmediatamente (OK)
```

**Latencia**: Microsegundos (solo validación + envío a canal)

### Escenario 2: Buffer Parcialmente Lleno

```
Sensor → Insertar() → [Canal casi lleno] → Espera < TimeoutBuffer → Retorna (OK)
```

**Latencia**: Variable (depende de velocidad de procesamiento)

### Escenario 3: Buffer Saturado

```
Sensor → Insertar() → [Canal lleno] → Espera TimeoutBuffer → Error timeout
```

**Error retornado:**
```
timeout (100ms): buffer saturado para serie sensor/temperatura
```

**Acción recomendada**: Reintentar con backoff exponencial o alertar al usuario

## Ajuste de Parámetros

### Cuándo aumentar TamañoBuffer

Aumentar el buffer cuando:

- ⚠️ **Errores frecuentes de timeout** en logs
- ⚠️ **Burst de datos predecibles**: Ej: sensores que reportan cada hora con 100 lecturas acumuladas
- ⚠️ **Múltiples series concurrentes**: Muchas series escribiendo simultáneamente

**Ejemplo:**
```go
// Para sistema con burst de hasta 500 lecturas cada 5 minutos
OpcionesEdge{
    TamañoBuffer: 1500,  // 3x margen de seguridad
}
```

**Consideraciones:**
- Más memoria RAM utilizada
- Mayor latencia en cierre (drain del buffer)

### Cuándo aumentar TimeoutBuffer

Aumentar el timeout cuando:

- ⚠️ **Hardware lento**: Dispositivos embebidos con CPU limitada
- ⚠️ **Compresión costosa**: Algoritmos como ZSTD nivel alto
- ⚠️ **I/O lento**: Almacenamiento en SD card o red

**Ejemplo:**
```go
// Para Raspberry Pi Zero con SD card
OpcionesEdge{
    TimeoutBuffer: 500 * 1000 * 1000,  // 500ms
}
```

**Consideraciones:**
- Aplicación cliente espera más tiempo
- Posibles timeouts HTTP si se usa API REST

### Cuándo disminuir TamañoBuffer

Reducir el buffer cuando:

- ✅ **Memoria limitada**: Dispositivos embebidos con <512 MB RAM
- ✅ **Muchas series**: Cientos o miles de series simultáneas
- ✅ **Latencia crítica**: Necesidad de persistir datos rápidamente

**Ejemplo:**
```go
// Para sistema embebido con 256 MB RAM y 100 series
OpcionesEdge{
    TamañoBuffer: 100,  // 100 × 100 series = 10,000 mediciones totales
}
```

## Monitoreo y Diagnóstico

### Detección de Saturación

Los errores de timeout indican saturación del sistema:

```go
err := manager.Insertar("sensor/temp", timestamp, 25.5)
if err != nil {
    if strings.Contains(err.Error(), "timeout") {
        // Buffer saturado - posibles acciones:
        // 1. Reintentar con backoff
        // 2. Alertar al administrador
        // 3. Descartar dato (solo si no es crítico)
        log.Printf("WARN: Sistema saturado - %v", err)
    }
}
```

### Métricas Recomendadas

Para monitorear la salud del sistema de buffering:

1. **Tasa de errores de timeout**: Debe ser < 0.1%
2. **Latencia P95 de Insertar()**: Debe ser < 10ms en operación normal
3. **Memoria de buffers**: `NumSeries × TamañoBuffer × 24 bytes`

## Casos de Uso

### Caso 1: Sensor Industrial (Alta Frecuencia)

**Perfil:**
- 10 Hz (10 lecturas/segundo)
- Burst ocasional de hasta 100 lecturas
- Hardware: Raspberry Pi 4

**Configuración recomendada:**
```go
OpcionesEdge{
    TamañoBuffer:  500,   // Permite burst de 100 + margen
    TimeoutBuffer: 50000000, // 50ms (sistema rápido)
}
```

### Caso 2: Red de Sensores Agrícolas (Baja Frecuencia)

**Perfil:**
- 1 lectura cada 5 minutos
- 50 sensores
- Hardware: BeagleBone Black

**Configuración recomendada:**
```go
OpcionesEdge{
    TamañoBuffer:  100,   // Suficiente para tráfico bajo
    TimeoutBuffer: 200000000, // 200ms (margen conservador)
}
```

### Caso 3: Gateway IoT (Múltiples Protocolos)

**Perfil:**
- MQTT + CoAP + HTTP simultáneos
- 200+ series activas
- Burst impredecibles

**Configuración recomendada:**
```go
OpcionesEdge{
    TamañoBuffer:  2000,  // Buffer grande por variabilidad
    TimeoutBuffer: 150000000, // 150ms
}
```

## Filosofía de Diseño

SensorWave sigue el modelo de **bases de datos embebidas** como SQLite o DuckDB:

- ✅ **Configurabilidad**: El desarrollador conoce mejor su caso de uso
- ✅ **Sensible por defecto**: Valores default funcionan para el 80% de casos
- ✅ **Sin magia oculta**: Comportamiento predecible y documentado
- ✅ **Tradeoffs explícitos**: Memoria vs throughput vs latencia

## Limitaciones Conocidas

### Pérdida de Datos en Crash

⚠️ **Importante**: Datos en el buffer **NO están persistidos** hasta que se completa el bloque y se comprime.

**Escenario de pérdida:**
```
1. Buffer tiene 50 mediciones en memoria
2. Crash del proceso (kill -9, pérdida de energía, etc.)
3. Esas 50 mediciones se pierden
```

**Mitigación actual**: Usar bloques pequeños (TamañoBloque: 10-20) para persistir más frecuentemente.

**Roadmap futuro**: Modo prioritario con fsync inmediato (ver [Trabajo Futuro](#trabajo-futuro)).

### No hay Backpressure Automático

El sistema **NO ralentiza automáticamente** a los productores cuando hay saturación. Es responsabilidad de la aplicación:

- Manejar errores de timeout
- Implementar reintentos con backoff
- Alertar al usuario si es necesario

## Trabajo Futuro

### Modo Prioritario con fsync (Planificado)

Para series críticas que requieren durabilidad garantizada:

```go
// Propuesta (NO implementado aún)
Serie{
    Path:      "sensor_critico/temperatura",
    Prioridad: tipos.PrioridadAlta,  // fsync inmediato
    // ...
}
```

**Comportamiento esperado:**
- `PrioridadNormal`: Flujo actual (buffer → compresión → PebbleDB)
- `PrioridadAlta`: Escritura inmediata a PebbleDB con fsync (sin buffer)

**Tradeoff:**
- ✅ Durabilidad garantizada (sobrevive a crashes)
- ❌ Throughput ~10-100x menor
- ❌ Mayor latencia de escritura

## Referencias

- [Código fuente: edge/edge.go](../edge/edge.go)
- [Definición de tipos: tipos/garage.go](../tipos/garage.go)
- [Tests de rendimiento: test/edge/series/series_test.go](../test/edge/series/series_test.go)

## Preguntas Frecuentes

### ¿Por qué no usar un buffer global en vez de uno por serie?

**R:** Un buffer global crearía **contención** entre series. El diseño actual permite:
- Paralelismo real (goroutines independientes)
- Aislamiento de fallos (una serie lenta no afecta a otras)
- Escalabilidad lineal con número de cores

### ¿Qué pasa si cierro el ManagerEdge con datos en buffers?

**R:** El método `Cerrar()` envía señal de cierre pero **NO espera** a que se drenen los buffers. Los datos no persistidos se pierden. Si necesitas garantías:

```go
// Esperar antes de cerrar (solución temporal)
time.Sleep(1 * time.Second)  // Dar tiempo a comprimir bloques pendientes
manager.Cerrar()
```

### ¿Puedo cambiar TamañoBuffer en tiempo de ejecución?

**R:** **No**. El tamaño del buffer se fija al crear el `ManagerEdge`. Para cambiar la configuración:

1. Cerrar el manager actual
2. Crear uno nuevo con nuevos parámetros
3. Las series existentes en PebbleDB se cargan automáticamente

### ¿Cómo convierto milisegundos a nanosegundos para TimeoutBuffer?

**R:** Multiplicar por 1,000,000:

```go
// 250ms en nanosegundos
TimeoutBuffer: 250 * 1000 * 1000  // = 250,000,000 ns
```

O usar `time.Duration`:
```go
timeout := 250 * time.Millisecond
TimeoutBuffer: int64(timeout)  // Conversión automática a ns
```

---

**Versión**: 1.0  
**Última actualización**: Diciembre 2025  
**Mantenedor**: Cristian Biale (cbiale@fi.uba.ar)
