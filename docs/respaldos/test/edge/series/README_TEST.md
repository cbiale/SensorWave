# Test de Series Agrícolas con Compresión

## Descripción

Este archivo (`series_test.go`) contiene pruebas exhaustivas del sistema de series temporales con compresión automática, utilizando ejemplos del dominio agrícola.

## Objetivo del Test

Validar el funcionamiento completo del sistema de series temporales, incluyendo:

1. **Creación de series** de todos los tipos de datos disponibles
2. **Inserción masiva de datos** con patrones realistas
3. **Compresión automática** al alcanzar el límite de tamaño de bloque
4. **Consultas sobre datos mezclados** (comprimidos en disco + en memoria)
5. **Optimización de consultas** (skip de bloques irrelevantes)
6. **Integridad de datos** (verificación de orden y completitud)

## Series de Datos Agrícolas

El test crea 5 series que representan un sistema de monitoreo agrícola:

| Serie | Tipo de Dato | Algoritmo Bloque | Algoritmo Bytes | Tamaño Bloque | Descripción |
|-------|--------------|------------------|-----------------|---------------|-------------|
| `campo1/temperatura_suelo` | Real | LZ4 | DeltaDelta | 15 | Temperatura del suelo en °C |
| `campo1/humedad_suelo` | Real | ZSTD | Xor | 20 | Humedad del suelo en % |
| `campo1/nivel_agua` | Integer | Snappy | DeltaDelta | 10 | Nivel del tanque de agua en cm |
| `campo1/riego_activo` | Boolean | LZ4 | RLE | 25 | Estado del sistema de riego |
| `campo1/estado_cultivo` | Text | ZSTD | Diccionario | 12 | Estado del cultivo (categórico) |

## Patrones de Datos

### Temperatura del Suelo (50 valores)
- **Patrón**: Variación diurna (15-30°C)
- **Resultado**: 3 bloques comprimidos + 5 valores en memoria
- **Compresión**: DeltaDelta (óptimo para tendencias graduales)

### Humedad del Suelo (60 valores)
- **Patrón**: Ciclos de riego (85% → 40% → 85%)
- **Resultado**: 3 bloques comprimidos (sin datos en memoria)
- **Compresión**: Xor (óptimo para cambios pequeños en flotantes)

### Nivel de Agua (35 valores)
- **Patrón**: Consumo gradual + rellenos periódicos
- **Resultado**: 3 bloques comprimidos + 5 valores en memoria
- **Compresión**: DeltaDelta (óptimo para enteros con tendencia)

### Riego Activo (75 valores)
- **Patrón**: ON (5 min) / OFF (10 min) cíclico
- **Resultado**: 3 bloques comprimidos
- **Compresión**: RLE (óptimo para secuencias repetidas)

### Estado del Cultivo (40 valores)
- **Patrón**: Categórico (`optimo`, `necesita_agua`, `necesita_nutrientes`, `alerta_plaga`)
- **Resultado**: 3 bloques comprimidos + 4 valores en memoria
- **Compresión**: Diccionario (óptimo para vocabulario limitado)

## Pruebas Realizadas

### 1. Creación de Series
Verifica que todas las series se creen correctamente con sus configuraciones de compresión.

### 2. Inserción de Datos
Inserta suficientes datos para provocar compresión automática al alcanzar el `TamañoBloque`.

### 3. Verificación de Series
Confirma que todas las series existen en el sistema.

### 4. Consultas de Rango
Realiza consultas sobre rangos temporales que incluyen:
- Datos comprimidos almacenados en disco
- Datos en memoria no comprimidos
- Análisis estadístico (promedio, mín, máx, distribuciones)

### 5. Consulta de Último Punto
Verifica que se pueda obtener eficientemente el último valor insertado (desde memoria).

### 6. Optimización de Skip
Valida que las consultas de rangos pequeños solo descompriman los bloques necesarios.

### 7. Integridad de Datos
Verifica:
- Orden cronológico correcto
- Ausencia de valores nulos
- Tipos de datos correctos
- Completitud de los datos

### 8. Resumen de Compresión
Presenta una tabla resumen del sistema de compresión utilizado.

## Tests Adicionales

### TestCompresionAutomatica
Verifica que la compresión se active automáticamente al alcanzar el tamaño de bloque configurado.

### TestRendimientoCompresion
Mide el rendimiento de diferentes algoritmos de compresión:
- LZ4 + DeltaDelta
- ZSTD + Xor
- Snappy + DeltaDelta
- Gzip + DeltaDelta

Métricas:
- Tiempo de inserción (1000 valores)
- Tiempo de consulta
- Número de bloques creados

## Ejecución

```bash
# Ejecutar solo el test principal
go test -v -run TestSeriesAgricolasCompresion

# Ejecutar todos los tests
go test -v

# Ejecutar con timeout extendido
go test -v -timeout 60s

# Ejecutar tests de rendimiento (skip en modo short)
go test -v -run TestRendimientoCompresion
```

## Resultados Esperados

✅ Compresión automática funcionando correctamente  
✅ Consultas eficientes sobre datos comprimidos + memoria  
✅ Optimización de skip de bloques operativa  
✅ Integridad de datos preservada  
✅ Todos los tipos de datos (Real, Integer, Boolean, Text) funcionando  

## Notas para la Tesis

Este test es ideal para el capítulo "Ensayos y Resultados" porque:

1. **Demuestra funcionalidad completa**: Cubre todos los aspectos del sistema
2. **Casos de uso realistas**: Ejemplos del dominio agrícola
3. **Métricas cuantificables**: Estadísticas, tiempos, tasas de compresión
4. **Validación de diseño**: Comprueba decisiones arquitectónicas
5. **Formato reproducible**: Tests automatizados y documentados

### Secciones Sugeridas para la Tesis

1. **Pruebas Funcionales**
   - Creación de series con diferentes configuraciones
   - Inserción y consulta de datos
   
2. **Pruebas de Compresión**
   - Activación automática
   - Tasas de compresión por algoritmo
   - Combinación de datos comprimidos y en memoria

3. **Pruebas de Rendimiento**
   - Latencia de inserción
   - Latencia de consulta
   - Eficiencia de skip de bloques

4. **Pruebas de Integridad**
   - Preservación de orden temporal
   - Ausencia de pérdida de datos
   - Correctitud de tipos
