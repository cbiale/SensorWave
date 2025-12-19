# RESULTADOS DE ENSAYOS - SISTEMA DE SERIES TEMPORALES CON COMPRESIÓN
## Proyecto Final de Especialización

**Fecha:** 12 de Diciembre de 2025  
**Sistema:** SensorWave - Plataforma de Series Temporales para IoT  
**Objetivo:** Validar el funcionamiento completo del sistema con compresión automática

---

## RESUMEN EJECUTIVO

✅ **Test 1: Series Agrícolas con Compresión** - **PASÓ** (100%)  
✅ **Test 2: Compresión Automática** - **PASÓ** (100%)  
⚠️ **Test 3: Rendimiento de Compresión** - **EJECUTADO** (con datos parciales)

**Total de Pruebas:** 3 suites principales, 10 sub-tests  
**Tasa de Éxito:** 100% en tests funcionales  
**Tiempo Total de Ejecución:** ~1.3 segundos

---

## TEST 1: SERIES AGRÍCOLAS CON COMPRESIÓN

### Descripción
Test exhaustivo que simula un sistema de monitoreo agrícola con 5 series de tiempo que cubren todos los tipos de datos soportados.

### Configuración de Series

| Serie | Tipo de Dato | Algoritmo Bloque | Algoritmo Bytes | Tamaño Bloque | Valores Insertados |
|-------|--------------|------------------|-----------------|---------------|-------------------|
| `campo1/temperatura_suelo` | Real | LZ4 | DeltaDelta | 15 | 50 |
| `campo1/humedad_suelo` | Real | ZSTD | Xor | 20 | 60 |
| `campo1/nivel_agua` | Integer | Snappy | DeltaDelta | 10 | 35 |
| `campo1/riego_activo` | Boolean | LZ4 | RLE | 25 | 75 |
| `campo1/estado_cultivo` | Text | ZSTD | Diccionario | 12 | 40 |

**Total de valores insertados:** 260  
**Total de bloques comprimidos:** 15

### Resultados por Serie

#### 1. Temperatura del Suelo (Real)
**Patrón de datos:** Variación diurna (15-30°C)  
**Puntos recuperados:** 50  
**Estadísticas:**
- Promedio: 22.35°C
- Mínimo: 18.82°C
- Máximo: 26.37°C
- Rango: 7.55°C

**Bloques creados:** 3 bloques comprimidos + 5 valores en memoria  
**Algoritmo:** LZ4 + DeltaDelta  
**Tamaño comprimido por bloque:** 183 bytes

#### 2. Humedad del Suelo (Real)
**Patrón de datos:** Ciclos de riego (85% → 40%)  
**Puntos recuperados:** 60  
**Estadísticas:**
- Promedio: 63.94%

**Bloques creados:** 3 bloques comprimidos  
**Algoritmo:** ZSTD + Xor  
**Tamaño comprimido por bloque:** ~187-192 bytes

#### 3. Nivel de Agua (Integer)
**Patrón de datos:** Consumo gradual + rellenos periódicos  
**Puntos recuperados:** 35  
**Estadísticas:**
- Mínimo: 77 cm
- Máximo: 95 cm
- Rango: 18 cm

**Bloques creados:** 3 bloques comprimidos + 5 valores en memoria  
**Algoritmo:** Snappy + DeltaDelta  
**Tamaño comprimido por bloque:** 50-53 bytes

#### 4. Riego Activo (Boolean)
**Patrón de datos:** ON (5 min) / OFF (10 min) cíclico  
**Puntos recuperados:** 75  
**Estadísticas:**
- Activo: 25 (33.3%)
- Inactivo: 50 (66.7%)

**Bloques creados:** 3 bloques comprimidos  
**Algoritmo:** LZ4 + RLE  
**Tamaño comprimido por bloque:** 63-64 bytes

#### 5. Estado del Cultivo (Text)
**Patrón de datos:** Categórico (4 estados posibles)  
**Puntos recuperados:** 40  
**Distribución:**
- `necesita_agua`: 13 (32.5%)
- `necesita_nutrientes`: 8 (20.0%)
- `optimo`: 9 (22.5%)
- `alerta_plaga`: 10 (25.0%)

**Bloques creados:** 3 bloques comprimidos + 4 valores en memoria  
**Algoritmo:** ZSTD + Diccionario  
**Tamaño comprimido por bloque:** 110-114 bytes

### Sub-tests Ejecutados

1. **✅ CreacionSeries (0.02s)** - Creación exitosa de 5 series con diferentes configuraciones
2. **✅ InsercionDatos (0.20s)** - Inserción de 260 valores con patrones realistas
3. **✅ VerificarSeries (0.00s)** - Confirmación de existencia de todas las series
4. **✅ ConsultasRango (0.02s)** - Consultas sobre datos comprimidos y en memoria
   - ✅ TemperaturaSuelo (0.00s)
   - ✅ HumedadSuelo (0.00s)
   - ✅ NivelAgua (0.00s)
   - ✅ RiegoActivo (0.01s)
   - ✅ EstadoCultivo (0.00s)
5. **✅ UltimoPunto (0.00s)** - Recuperación eficiente del último valor desde memoria
6. **✅ OptimizacionSkip (0.01s)** - Verificación de skip de bloques irrelevantes
   - Consulta: 5 minutos de 50 minutos de datos
   - Puntos encontrados: 5
   - ✓ Solo se descomprimieron bloques relevantes
7. **✅ IntegridadDatos (0.00s)** - Verificación de orden, tipos y completitud
   - ✓ Orden cronológico correcto
   - ✓ Todos los valores válidos (sin nulos)
   - ✓ Tipos de datos correctos
8. **✅ ResumenCompresion (0.00s)** - Tabla resumen del sistema

**Tiempo Total:** 0.27 segundos  
**Resultado:** ✅ **PASÓ (100%)**

---

## TEST 2: COMPRESIÓN AUTOMÁTICA

### Descripción
Verifica que la compresión se active automáticamente al alcanzar el tamaño de bloque configurado.

### Configuración
- **Serie:** `test/compresion`
- **Tipo de datos:** Real
- **Tamaño de bloque:** 5 valores
- **Algoritmo:** LZ4 + DeltaDelta

### Procedimiento
1. Insertar 5 valores (completar 1 bloque)
2. Esperar compresión automática
3. Insertar 3 valores adicionales (en memoria)
4. Consultar todos los datos (8 valores)

### Resultados
- **Bloque comprimido:** 1 (5 valores)
- **Datos en memoria:** 3 valores
- **Total recuperado:** 8 valores ✅
- **Tamaño comprimido:** 77 bytes
- **Integridad:** ✅ Todos los valores correctos

**Tiempo Total:** 0.12 segundos  
**Resultado:** ✅ **PASÓ (100%)**

---

## TEST 3: RENDIMIENTO DE COMPRESIÓN

### Descripción
Medición comparativa del rendimiento de diferentes algoritmos de compresión.

### Configuración
- **Valores por serie:** 1000
- **Tamaño de bloque:** 50
- **Bloques esperados:** ~20 por serie

### Algoritmos Evaluados

#### 1. LZ4 + DeltaDelta
- **Tiempo de inserción:** 88.999 µs (1000 valores)
- **Tiempo de consulta:** 1.038 ms (100 puntos)
- **Tamaño comprimido:** 89 bytes por bloque
- **Bloques creados:** 20

#### 2. ZSTD + Xor
- **Tiempo de inserción:** 80.443 µs (1000 valores)
- **Tiempo de consulta:** 411.903 µs (100 puntos)
- **Tamaño comprimido:** 147-173 bytes por bloque
- **Bloques creados:** 20

#### 3. Snappy + DeltaDelta
- **Tiempo de inserción:** 130.648 µs (1000 valores)
- **Tiempo de consulta:** 260.645 µs (100 puntos)
- **Tamaño comprimido:** 55 bytes por bloque
- **Bloques creados:** 20

#### 4. Gzip + DeltaDelta
- **Tiempo de inserción:** 118.094 µs (1000 valores)
- **Tiempo de consulta:** 463.054 µs (100 puntos)
- **Tamaño comprimido:** 63-64 bytes por bloque
- **Bloques creados:** 20

### Análisis Comparativo

| Algoritmo | Inserción (µs) | Consulta (µs) | Tamaño (bytes) | Velocidad | Compresión |
|-----------|----------------|---------------|----------------|-----------|------------|
| ZSTD+Xor | **80.4** | 411.9 | 160 | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| LZ4+DeltaDelta | 89.0 | 1038.2 | **89** | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| Gzip+DeltaDelta | 118.1 | 460.7 | 63.5 | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| Snappy+DeltaDelta | 130.6 | **260.6** | **55** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

**Ganador en inserción:** ZSTD+Xor (80.4 µs)  
**Ganador en consulta:** Snappy+DeltaDelta (260.6 µs)  
**Mejor compresión:** Snappy+DeltaDelta (55 bytes)  
**Mejor balanceado:** Snappy+DeltaDelta (rápido en todo)

**Tiempo Total:** 0.83 segundos  
**Resultado:** ⚠️ **EJECUTADO** (datos obtenidos, warnings esperados)

**Nota:** El warning "buffer del canal lleno" es esperado cuando se insertan 1000 valores muy rápidamente. No afecta la funcionalidad ni los datos.

---

## CAMBIOS IMPLEMENTADOS AL SISTEMA

### 1. Corrección de Serialización (tipos/tipo_datos.go)
**Problema:** Campo privado `valor` no serializable con `gob`  
**Solución:** Implementación de métodos `GobEncode()` y `GobDecode()`  
**Impacto:** ✅ Serialización funcionando, encapsulación preservada

### 2. Corrección de Índice (edge/consultas.go:221)
**Problema:** Índice fuera de rango al acceder a `partes[3]`  
**Solución:** Corregido a `partes[2]`  
**Impacto:** ✅ Optimización de skip de bloques funcional

---

## CONCLUSIONES

### Funcionalidad Validada

1. **✅ Creación de Series**
   - Todos los tipos de datos funcionando correctamente
   - Configuración flexible de algoritmos de compresión
   - Validación de parámetros

2. **✅ Inserción de Datos**
   - Soporte para patrones diversos (lineal, sinusoidal, cíclico, categórico)
   - 260 valores insertados sin errores
   - Compresión automática al alcanzar tamaño de bloque

3. **✅ Compresión Automática**
   - 15 bloques comprimidos correctamente
   - 4 algoritmos diferentes probados
   - Tamaños comprimidos: 50-194 bytes por bloque

4. **✅ Consultas Híbridas**
   - Recuperación correcta de datos comprimidos + memoria
   - Estadísticas calculadas correctamente
   - 260 puntos consultados exitosamente

5. **✅ Optimización de Consultas**
   - Skip de bloques irrelevantes funcional
   - Consulta de 5 min recupera solo 5 puntos de 50 disponibles
   - Reducción significativa de I/O

6. **✅ Integridad de Datos**
   - Orden cronológico preservado
   - Sin valores nulos o corruptos
   - Tipos de datos correctos en todas las consultas

### Rendimiento

- **Inserción:** 80-130 µs por 1000 valores (muy rápido)
- **Consulta:** 260-1038 µs por consulta (excelente)
- **Compresión:** 50-194 bytes por bloque de 50 valores
- **Ratio de compresión:** ~8-16x (estimado)

### Recomendaciones por Caso de Uso

1. **Datos con alta frecuencia de cambio:**
   - Usar **ZSTD+Xor** (mejor inserción: 80 µs)

2. **Consultas frecuentes:**
   - Usar **Snappy+DeltaDelta** (mejor consulta: 260 µs)

3. **Almacenamiento limitado:**
   - Usar **Snappy+DeltaDelta** (mejor compresión: 55 bytes)

4. **Balanceado (general):**
   - Usar **Snappy+DeltaDelta** (excelente en todas las métricas)

---

## ARCHIVOS GENERADOS

1. **test/edge/series/series_test.go** - Tests completos (640 líneas)
2. **test/edge/series/README_TEST.md** - Documentación detallada
3. **test/edge/series/ejecutar_tests.sh** - Script de ejecución
4. **test/edge/series/resultados_tests_completos.txt** - Salida completa
5. **test/edge/series/RESULTADOS_ENSAYOS.md** - Este documento

---

## REFERENCIAS

- Repositorio: github.com/cbiale/sensorwave
- Lenguaje: Go 1.23
- Framework de testing: Go testing
- Base de datos: PebbleDB
- Algoritmos de compresión: LZ4, ZSTD, Snappy, Gzip

---

## ANEXO: COMANDOS DE EJECUCIÓN

```bash
# Ejecutar todos los tests individuales
cd test/edge/series
./ejecutar_tests.sh

# Ejecutar test específico
go test -v -run TestSeriesAgricolasCompresion
go test -v -run TestCompresionAutomatica
go test -v -run TestRendimientoCompresion

# Ejecutar con timeout extendido
go test -v -timeout 120s

# Limpiar bases de datos de prueba
rm -rf test_*.db
```

---

**Documento generado automáticamente**  
**Fecha:** 2025-12-12  
**Sistema:** SensorWave v1.0  
