# Algoritmos de Compresión por Tipo de Dato

## Resumen Ejecutivo

**Recomendación: SÍ adaptar el sistema a tipos específicos**

El sistema actual usa `interface{}` y agrupa todos los numéricos juntos, lo que resulta en compresión subóptima. La adaptación a tipos específicos puede mejorar la compresión 2-5x para ciertos datos.

## Estado Actual

### Problemas del diseño actual:
- `interface{}` para valores (tipos/medicion.go:6)
- Todos los numéricos tratados igual (edge/utils.go:121-122)
- Booleanos comprimidos como categóricos/numéricos
- Sin distinción entre float32/float64 o int32/int64

## Propuesta de Arquitectura

### Nuevos tipos de datos:
```go
type TipoDato string
const (
    Float64    TipoDato = "FLOAT64"
    Float32    TipoDato = "FLOAT32" 
    Int64      TipoDato = "INT64"
    Int32      TipoDato = "INT32"
    Boolean    TipoDato = "BOOLEAN"
    String     TipoDato = "STRING"
)
```

### Compresores especializados:
```go
type CompresorEspecializado interface {
    Comprimir(datos []interface{}) ([]byte, error)
    Descomprimir(datos []byte) ([]interface{}, error)
}
```

## Algoritmos Recomendados por Tipo

### 1. Float64 (Datos de sensores: temperatura, presión, humedad)

**Algoritmo: Gorilla (XOR + Delta-of-Delta)**
- **Ratio compresión**: 5-15x mejor que sin compresión
- **Velocidad**: Alta
- **Uso ideal**: Series temporales con cambios pequeños entre valores

**Implementación:**
- Primer valor: 64 bits sin comprimir
- Valores idénticos: 1 bit (0)
- Valores diferentes: 1 bit (1) + bits significativos adaptativos
- Manejo optimizado de leading/trailing zeros

### 2. Float32 (Sensores de baja precisión)

**Algoritmo: Gorilla adaptado 32-bit**
- **Ratio compresión**: 3-10x
- **Velocidad**: Alta
- **Uso ideal**: IoT con memoria limitada

**Adaptaciones:**
- Primer valor: 32 bits
- Control de bits ajustado a 32 bits
- Menor uso de memoria

### 3. Int64 (Timestamps, contadores grandes)

**Algoritmo: Simple-8b + Delta**
- **Ratio compresión**: 2-8x
- **Velocidad**: Muy alta
- **Uso ideal**: Timestamps, IDs únicos, contadores

**Características:**
- Compresión de longitud variable (1-60 bits por entero)
- Delta encoding para valores consecutivos
- Excelente para datos monotónicos

### 4. Int32 (Contadores, IDs pequeños)

**Algoritmo: Simple-8b + Delta**
- **Ratio compresión**: 2-6x
- **Velocidad**: Muy alta
- **Uso ideal**: Contadores de eventos, IDs de dispositivo

### 5. Boolean (Estados, flags, alarmas)

**Algoritmo: RLE bit-packed**
- **Ratio compresión**: 8-64x (dependiendo de patrones)
- **Velocidad**: Extrema
- **Uso ideal**: Estados ON/OFF, alarmas, flags

**Implementación:**
- 8 booleanos por byte
- RLE para secuencias largas
- Header con longitud de runs

### 6. String (Categorías, mensajes, logs)

**Algoritmo: Huffman Coding + Dictionary**
- **Ratio compresión**: 3-12x
- **Velocidad**: Media
- **Uso ideal**: Categorías repetitivas, logs estructurados

**Estrategia:**
- Dictionary para valores frecuentes
- Huffman para remaining data
- Cache de diccionarios por serie

## Comparación de Rendimiento

| Tipo | Algoritmo Actual | Algoritmo Recomendado | Mejora |
|------|------------------|----------------------|---------|
| Float64 | XOR básico | Gorilla | 30-50% |
| Float32 | XOR básico | Gorilla-32 | 25-40% |
| Int64 | XOR básico | Simple-8b | 40-60% |
| Int32 | XOR básico | Simple-8b | 35-55% |
| Boolean | RLE genérico | RLE bit-packed | 200-500% |
| String | Sin compresión | Huffman+Dict | 300-1200% |

## Plan de Implementación

### Fase 1: Infraestructura (1-2 semanas)
1. Extender `tipos.TipoDatos` con tipos específicos
2. Crear interfaces `CompresorEspecializado`
3. Implementar factories por tipo
4. Actualizar `inferirTipo()` para detección precisa

### Fase 2: Compresores Numéricos (2-3 semanas)
1. Implementar Gorilla completo para Float64/Float32
2. Implementar Simple-8b para Int64/Int32
3. Optimizar delta encoding
4. Testing y benchmarking

### Fase 3: Compresores Especializados (2 semanas)
1. Implementar RLE bit-packed para Boolean
2. Implementar Huffman + Dictionary para String
3. Optimización de memoria
4. Testing con datos reales

### Fase 4: Integración (1 semana)
1. Actualizar sistema de compresión existente
2. Migración de datos compatibles
3. Actualizar APIs y configuración
4. Documentación y ejemplos

## Impacto en Sistema Existente

### Cambios requeridos:
- `tipos/medicion.go`: Valor interface{} → tipo específico
- `edge/utils.go`: `inferirTipo()` actualizado
- `compresor/`: Nuevos compresores especializados
- APIs: Configuración por tipo de dato

### Compatibilidad:
- **Backward**: Datos existentes migrables
- **Forward**: Nuevos tipos soportados
- **Rendimiento**: Mejora general sin degradación

## Métricas Esperadas

### Compresión:
- **Reducción almacenamiento**: 40-70% promedio
- **Mejora transmisión**: 50-80% menos datos
- **CPU overhead**: <5% adicional

### Memoria:
- **Uso RAM**: Reducción 20-40% en buffers
- **Cache**: Mejor localidad por tipo
- **GC pressure**: Menor por datos compactos

## Casos de Uso IoT

### Sensores Ambientales:
- Temperatura: Float64 + Gorilla
- Humedad: Float32 + Gorilla-32
- Presión: Float64 + Gorilla

### Dispositivos Industriales:
- Estado: Boolean + RLE bit-packed
- Contador: Int32 + Simple-8b
- Timestamp: Int64 + Simple-8b + Delta

### Logs y Eventos:
- Nivel evento: String + Huffman
- Código error: String + Dictionary
- Timestamp: Int64 + Simple-8b

## Conclusión

La adaptación a tipos específicos es **altamente recomendable** con beneficios claros:

1. **Mejor compresión**: 2-5x para booleanos, 30-60% para numéricos
2. **Menor almacenamiento**: Reducción 40-70% en uso de disco
3. **Mejor rendimiento**: Transmisión 50-80% más rápida
4. **Escalabilidad**: Soporte para más tipos de datos IoT

El esfuerzo de implementación (6-8 semanas) se justifica por los beneficios a largo plazo en eficiencia y escalabilidad del sistema.