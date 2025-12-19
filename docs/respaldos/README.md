# Tests del Motor de Reglas - SensorWave

Este directorio contiene los tests completos del Motor de Reglas de SensorWave, incluyendo soporte multi-tipo y agregaciones con filtros.

## Archivos

- **`reglas_test.go`**: Suite completa de tests (1557 líneas, 25 test cases)
- **`ejecutar_tests.sh`**: Script bash para ejecutar todos los tests con output formateado

## Ejecutar Tests

### Opción 1: Usando el script (recomendado)

```bash
./ejecutar_tests.sh
```

El script ejecuta automáticamente todos los tests y muestra:
- Progreso en tiempo real con códigos de color
- Resumen por sección (tipos básicos, mixtos, agregaciones, validaciones)
- Estadísticas finales (total, pasados, fallados, duración)
- Lista completa de funcionalidades validadas

### Opción 2: Usando go test directamente

Ejecutar todos los tests:
```bash
go test -v
```

Ejecutar un test específico:
```bash
go test -v -run TestReglasBoolean
go test -v -run TestReglasText
go test -v -run TestAgregacionCountConFiltro
```

Ejecutar con timeout personalizado:
```bash
go test -v -timeout 10m
```

## Estructura de Tests

### Sección 1: Tipos de Datos Básicos (3 tests)
- **TestReglasBoolean**: Valores true/false con operadores == y !=
- **TestReglasInteger**: Valores int64 con todos los operadores numéricos
- **TestReglasReal**: Valores float64 con todos los operadores numéricos

### Sección 2: Tipos Mixtos y Lógica (2 tests)
- **TestReglasMixtos**: Reglas combinando Boolean + Integer + Real
- **TestReglasOperadoresLogicos**: Lógica AND y OR entre condiciones

### Sección 3: Tipo Text (1 test)
- **TestReglasText**: Comparación case-insensitive de strings (3 subtests)
  - Igualdad exacta
  - Case-insensitive matching
  - Operador distinto (!=)

### Sección 4: Agregaciones con Filtro (2 tests)
- **TestAgregacionCountConFiltro**: count(WHERE valor == filtro) (5 subtests)
  - Boolean con filtro true/false
  - String con filtro case-insensitive
  - Integer con filtro numérico
- **TestAgregacionCountSinFiltro**: count() sin filtro (2 subtests)
  - Boolean contando todos los valores
  - Text contando todos los valores

### Sección 5: Validaciones (1 test)
- **TestValidacionTipos**: Errores de tipos incompatibles (7 subtests)
  - Boolean con operador numérico (error)
  - String con operador numérico (error)
  - Boolean con agregación promedio (error)
  - String con agregación suma (error)
  - Filtro con agregación no-count (error)
  - Boolean con == y != (válido)
  - Boolean con count (válido)

## Funcionalidades Validadas

### 1. Tipos de Datos
✓ **Boolean** (true/false) - operadores: `==`, `!=`  
✓ **Integer** (int64) - operadores: `==`, `!=`, `>`, `>=`, `<`, `<=`  
✓ **Real** (float64) - operadores: `==`, `!=`, `>`, `>=`, `<`, `<=`  
✓ **Text** (string) - operadores: `==`, `!=` (case-insensitive)

### 2. Agregaciones
✓ `count()` - todos los tipos  
✓ `suma`, `promedio`, `maximo`, `minimo` - solo numéricos  
✓ Validación de tipos incompatibles

### 3. Filtros en Agregaciones
✓ `count(WHERE valor == filtro)` - Boolean  
✓ `count(WHERE valor == filtro)` - Integer  
✓ `count(WHERE valor == filtro)` - String (case-insensitive)  
✓ `count()` sin filtro - cuenta todos los valores

### 4. Validaciones
✓ Operadores incompatibles con tipos  
✓ Agregaciones incompatibles con tipos  
✓ Filtros solo con agregación count  
✓ Operadores válidos para cada tipo

### 5. Lógica de Reglas
✓ AND - todas las condiciones deben cumplirse  
✓ OR - al menos una condición debe cumplirse  
✓ Reglas con tipos mixtos

## Ejemplos de Uso

### Boolean
```go
Condicion{
    Serie:    "sistema/activo",
    Operador: edge.OperadorIgual,
    Valor:    true,  // Direct boolean
    VentanaT: 10 * time.Second,
}
```

### String (case-insensitive)
```go
Condicion{
    Serie:    "sensor/estado",
    Operador: edge.OperadorIgual,
    Valor:    "ERROR",  // Matches "error", "Error", "ERROR"
    VentanaT: 10 * time.Second,
}
```

### Count con filtro
```go
Condicion{
    SeriesGrupo: []string{"sensor/s1", "sensor/s2", "sensor/s3"},
    Agregacion:  edge.AgregacionCount,
    FiltroValor: true,  // Solo cuenta valores true
    Operador:    edge.OperadorMayorIgual,
    Valor:       int64(2),  // Al menos 2 sensores en true
    VentanaT:    10 * time.Second,
}
```

### Count sin filtro
```go
Condicion{
    SeriesGrupo: []string{"server/s1", "server/s2"},
    Agregacion:  edge.AgregacionCount,
    FiltroValor: nil,  // Cuenta todos los valores
    Operador:    edge.OperadorIgual,
    Valor:       int64(2),  // Exactamente 2 servidores reportando
    VentanaT:    10 * time.Second,
}
```

## Tiempo de Ejecución

**Típico**: ~30-40 segundos para ejecutar todos los 25 test cases

**Desglose por sección**:
- Tipos básicos: ~0.7s
- Tipos mixtos y lógica: ~0.3s
- Text: ~0.3s
- Agregaciones con filtro: ~0.6s
- Agregaciones sin filtro: ~0.2s
- Validaciones: ~0.06s

## Troubleshooting

### Error: "bind: address already in use"
Algunos tests pueden fallar si hay conflicto de puertos HTTP.

**Solución**:
```bash
# Ejecutar tests con delay entre ellos
go test -v -run TestReglasBoolean
sleep 1
go test -v -run TestReglasInteger
# ... etc
```

O usar el script `ejecutar_tests.sh` que maneja esto automáticamente.

### Limpiar bases de datos de test
```bash
rm -rf test_*.db
```

El script `ejecutar_tests.sh` hace esto automáticamente antes de cada ejecución.

## Cambios Importantes (Breaking Changes)

Este motor de reglas tiene cambios incompatibles con versiones anteriores:

1. **`Condicion.Valor`** cambió de `float64` a `interface{}`
   - Antes: `Valor: 1.0` (para boolean true)
   - Ahora: `Valor: true` (boolean directo)

2. **`EjecutorAccion`** cambió la firma
   - Antes: `func(Accion, *Regla, map[string]float64) error`
   - Ahora: `func(Accion, *Regla, map[string]interface{}) error`

3. **Reglas existentes en PebbleDB** no son compatibles
   - Solución: Limpiar la base de datos de desarrollo

## Más Información

Ver archivo principal del motor de reglas:
- `../../../edge/reglas.go` - Implementación completa

Ver documentación adicional:
- `../../../docs/respaldos/` - Documentación de diseño e implementación
