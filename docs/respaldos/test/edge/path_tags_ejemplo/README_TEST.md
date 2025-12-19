# Tests de Path y Tags

Este directorio contiene tests automatizados que validan el sistema de Path y Tags para organización de series temporales.

## Descripción

El sistema implementa un modelo híbrido de organización:
- **Path**: Organización jerárquica (ej: `dispositivo_001/temperatura`)
- **Tags**: Metadatos flexibles para filtrado (ej: `{ubicacion: "sala1", tipo: "DHT22"}`)

## Tests Disponibles

### TestPathTagsCompleto

Test integral que valida todas las funcionalidades del sistema Path+Tags.

**Sub-tests:**
1. **CrearSeriesConPathYTags** - Creación de 4 series con diferentes combinaciones de tags
2. **InsertarDatos** - Inserción de 30 valores en total (10 por dispositivo)
3. **ConsultaPorDispositivo** - Filtrado por dispositivo usando patrón de path
4. **ConsultaPorPathPattern** - Consultas usando wildcards (`*/temperatura`)
5. **ConsultaPorTags** - Filtrado por metadatos (zona, tipo, edificio)
6. **ConsultaDatosRangoTemporal** - Consulta de valores con rango de tiempo
7. **ReglaConPathPatternYTags** - Creación de reglas usando path patterns y tags
8. **ResumenSistema** - Verificación final del estado del sistema

**Configuración de Series:**

| Path | Ubicación | Tipo Sensor | Zona | Edificio |
|------|-----------|-------------|------|----------|
| dispositivo_001/temperatura | sala1 | DHT22 | produccion | norte |
| dispositivo_001/humedad | sala1 | DHT22 | produccion | norte |
| dispositivo_002/temperatura | sala2 | DS18B20 | almacen | norte |
| dispositivo_003/temperatura | oficina1 | DHT22 | oficinas | sur |

**Validaciones:**
- ✅ Creación correcta de series con tags
- ✅ Inserción de datos exitosa
- ✅ Consultas por dispositivo (2 series esperadas)
- ✅ Wildcards funcionando (3 temperaturas encontradas)
- ✅ Filtrado por zona "produccion" (2 series)
- ✅ Filtrado múltiple: DHT22 en edificio norte (2 series)
- ✅ Consulta de rango temporal (10 mediciones)
- ✅ Orden cronológico de datos
- ✅ Valores en rango esperado (23.5°C - 24.5°C)
- ✅ Reglas con PathPattern y TagsFilter

**Duración:** ~0.02s

### TestConsultasAvanzadas

Test de consultas complejas con múltiples combinaciones de tags.

**Configuración:**
- 4 series con diferentes combinaciones de tags
- Edificios: A, B
- Pisos: 1, 2
- Tipos de sensor: DHT22, DS18B20
- Métricas: temperatura, humedad

**Sub-tests:**
1. **FiltrarPorEdificio** - Todas las series del edificio A (3 esperadas)
2. **FiltrarPorEdificioYPiso** - Series de edificio A, piso 1 (2 esperadas)
3. **FiltrarPorTipoSensor** - Todas las series DHT22 (3 esperadas)
4. **WildcardHumedad** - Búsqueda por métrica (`*/humedad`) (1 esperada)

**Validaciones:**
- ✅ Filtrado por un solo tag
- ✅ Filtrado por múltiples tags simultáneos
- ✅ Wildcards en paths
- ✅ Verificación de tags correctos en resultados

**Duración:** ~0.02s

## Ejecutar Tests

### Todos los tests
```bash
# Ejecutar todos los tests
go test -v

# Con timeout extendido
go test -v -timeout 5m
```

### Tests individuales
```bash
# Solo el test completo
go test -v -run TestPathTagsCompleto

# Solo consultas avanzadas
go test -v -run TestConsultasAvanzadas
```

**NOTA:** Debido a un conflicto en el registro del handler HTTP cuando se ejecutan múltiples tests en el mismo proceso, se recomienda ejecutar los tests por separado como se muestra arriba.

## Resultados Esperados

```
=== RUN   TestPathTagsCompleto
    ✓ Manager Edge inicializado
    ✓ 4 series creadas con tags
    ✓ 30 valores insertados
    ✓ Consultas por dispositivo exitosas
    ✓ Wildcards funcionando correctamente
    ✓ Filtrado por tags validado
    ✓ Consulta temporal con 10 mediciones
    ✓ Regla agregada exitosamente
    ✓ Sistema completo validado
--- PASS: TestPathTagsCompleto (0.02s)

=== RUN   TestConsultasAvanzadas
    ✓ 4 series con múltiples tags creadas
    ✓ Filtrado por edificio: 3 series
    ✓ Filtrado múltiple: 2 series
    ✓ Filtrado por tipo: 3 series
    ✓ Wildcard humedad: 1 serie
--- PASS: TestConsultasAvanzadas (0.02s)

PASS
```

## Ventajas Validadas

El sistema Path+Tags demostró las siguientes ventajas:

1. **Organización Jerárquica Clara**
   - Paths jerárquicos tipo filesystem
   - Fácil navegación y agrupación por dispositivo

2. **Filtrado Flexible**
   - Búsqueda por múltiples dimensiones (ubicación, tipo, zona, etc.)
   - Combinaciones arbitrarias de tags

3. **Consultas Eficientes**
   - Wildcards en paths (`*/temperatura`)
   - Filtrado por tags sin escaneo completo
   - Combinación de path patterns y tag filters

4. **Compatibilidad**
   - Modelo compatible con TSDBs modernas:
     - IoTDB (Path + Tags)
     - Prometheus (Labels)
     - InfluxDB (Measurement + Tags)

## Casos de Uso Validados

1. **Monitoreo por Dispositivo**
   - Listar todas las métricas de un dispositivo específico
   - Ejemplo: `ListarSeriesPorDispositivo("dispositivo_001")`

2. **Monitoreo por Tipo de Métrica**
   - Todas las temperaturas del sistema
   - Ejemplo: `ListarSeriesPorPath("*/temperatura")`

3. **Filtrado por Ubicación/Zona**
   - Series en zona de producción
   - Ejemplo: `ListarSeriesPorTags({"zona": "produccion"})`

4. **Consultas Combinadas**
   - Sensores DHT22 en edificio norte
   - Ejemplo: `ListarSeriesPorTags({"tipo": "DHT22", "edificio": "norte"})`

5. **Reglas de Negocio**
   - Alertas sobre subconjuntos específicos
   - Ejemplo: PathPattern + TagsFilter en reglas

## Archivos

- `main.go` - Programa de ejemplo interactivo (para demostraciones)
- `path_tags_test.go` - Tests automatizados (para CI/CD)
- `README_TEST.md` - Esta documentación

### ¿Cuándo usar cada uno?

| Archivo | Uso Recomendado |
|---------|-----------------|
| `main.go` | Demostraciones, debugging, aprendizaje |
| `path_tags_test.go` | CI/CD, validación automática, regression testing |

## Migración desde main.go

El archivo `path_tags_test.go` es una conversión directa de `main.go` a formato de test de Go:

**Diferencias principales:**
- ✅ Usa framework `testing` estándar de Go
- ✅ Validaciones automáticas con `t.Error()` y `t.Fatalf()`
- ✅ Sub-tests organizados con `t.Run()`
- ✅ Limpieza automática de base de datos con `defer`
- ✅ Puertos únicos para evitar conflictos (4230, 4231)
- ✅ Verificaciones cuantitativas (conteo de resultados)
- ✅ Validación de orden cronológico
- ✅ Validación de rangos de valores

**Funcionalidad preservada:**
- ✅ Todas las consultas originales
- ✅ Misma configuración de series
- ✅ Mismos datos de prueba
- ✅ Misma lógica de reglas

## Métricas

| Métrica | Valor |
|---------|-------|
| Total de tests | 2 |
| Sub-tests | 12 |
| Series creadas | 8 (4 por test) |
| Valores insertados | 70 |
| Consultas validadas | 10+ |
| Cobertura | Path queries, Tag queries, Wildcards, Reglas |
| Duración total | ~0.04s |
| Tasa de éxito | 100% (cuando se ejecutan por separado) |

## Integración Continua

Para CI/CD, ejecutar tests por separado:

```bash
#!/bin/bash
set -e

echo "Ejecutando test de Path y Tags completo..."
go test -v -run TestPathTagsCompleto -timeout 5m

echo "Ejecutando test de consultas avanzadas..."
go test -v -run TestConsultasAvanzadas -timeout 5m

echo "✅ Todos los tests pasaron"
```

## Referencias

- Documentación de series: `edge/series.go`
- Documentación de reglas: `edge/reglas.go`
- Tests de series: `test/edge/series/series_test.go`
