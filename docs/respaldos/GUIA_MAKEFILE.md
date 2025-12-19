# Guía Rápida del Makefile - SensorWave

## 📋 Introducción

Este documento explica cómo usar el Makefile del proyecto SensorWave para ejecutar tests, generar cobertura y realizar otras tareas comunes.

## 🚀 Inicio Rápido

### Ver Todos los Comandos Disponibles

```bash
make help
```

Esto muestra una lista completa de todos los comandos (targets) disponibles, organizados por categorías.

---

## 🧪 Testing

### Comandos Básicos

| Comando | Descripción | Tiempo Aprox. |
|---------|-------------|---------------|
| `make test` | Ejecuta todos los tests (modo rápido) | ~30s |
| `make test-coverage` | Tests + reporte HTML de cobertura | ~35s |
| `make test-all` | Tests usando script bash (output detallado) | ~36s |
| `make test-all-coverage` | Script bash + cobertura | ~40s |

### Tests por Suite

| Comando | Tests | Descripción |
|---------|-------|-------------|
| `make test-reglas` | 9 tests | Motor de reglas (tipos, agregaciones, validaciones) |
| `make test-series` | 6 tests | Series temporales (compresión, rendimiento) |
| `make test-path-tags` | 2 tests | Modelo path + tags |
| `make test-middleware` | Varios | Middleware (CoAP, HTTP, MQTT, NATS) |
| `make test-despachador` | Varios | Servicio despachador en la nube |

### Tests Individuales

#### Motor de Reglas

```bash
make test-reglas-boolean    # Reglas Boolean (true/false)
make test-reglas-integer    # Reglas Integer (int64)
make test-reglas-real       # Reglas Real (float64)
make test-reglas-text       # Reglas Text (case-insensitive)
```

#### Series Temporales

```bash
make test-series-compresion # Series agrícolas con compresión
make test-series-carga      # Carga masiva (10,000 valores)
```

---

## 📊 Cobertura de Código

### Generar Reporte de Cobertura HTML

```bash
make test-coverage
```

Esto genera:
- **Archivo HTML**: `test/coverage.html`
- **Archivo de datos**: `test/coverage.out`
- **Resumen en terminal**: Muestra porcentaje total de cobertura

**Paquetes analizados:**
- `edge` - Nodo al borde de la red
- `compresor` - Algoritmos de compresión
- `tipos` - Tipos de datos y estructuras
- `middleware/servidor` - Servidor middleware
- `despachador` - Servicio despachador

### Ver Reporte de Cobertura

**En navegador** (recomendado):
```bash
make coverage-html
```

Abre automáticamente `test/coverage.html` en tu navegador predeterminado.

**En terminal**:
```bash
make coverage-report
```

Muestra el reporte de cobertura por función directamente en la terminal.

### Interpretación del Reporte

El reporte HTML muestra:
- **Verde**: Líneas cubiertas por tests
- **Rojo**: Líneas NO cubiertas por tests
- **Gris**: Líneas no ejecutables (comentarios, llaves, etc.)

**Métricas**:
- ≥80% - Cobertura excelente (verde)
- 60-79% - Cobertura aceptable (amarillo)
- <60% - Cobertura baja (rojo)

---

## 📈 Gráficos para Tesis

### Generar Gráficos

```bash
make graficos
```

**Requisitos previos**:
```bash
pip install matplotlib pandas numpy
```

**Salida**:
- Genera 8 gráficos en formato PNG (300 DPI)
- Ubicación: `test/edge/series/graficos/`

**Gráficos generados**:
1. Evolución de humedad del suelo con riego
2. Temperatura ambiente del día
3. Análisis de compresión (ratios)
4. Rendimiento de inserción
5. Concurrencia
6. Comparativa de algoritmos
7. Arquitectura del sistema
8. Y más...

---

## 🔨 Construcción

### Compilar el Proyecto

```bash
make build
```

Compila todos los paquetes principales:
- `tipos`
- `compresor`
- `edge`
- `middleware`
- `despachador`

### Compilar Tests

```bash
make build-tests
```

Genera binarios ejecutables de los tests en:
- `test/edge/series/series.test`
- `test/edge/path_tags/path_tags.test`
- `test/edge/reglas/reglas.test`

---

## 🧹 Limpieza

### Limpiar Artefactos de Tests

```bash
make clean
```

Elimina:
- Bases de datos temporales (`test_*.db`)
- Archivos de cobertura (`coverage.out`, `coverage.html`)
- DBs de ejemplo

### Limpieza Completa

```bash
make clean-all
```

Elimina todo lo anterior más:
- Binarios de tests (`*.test`)
- Binarios compilados
- Cachés de Go

---

## 🐳 Infraestructura

### Iniciar Contenedores

```bash
make run-garage         # Garage (S3) - Puerto 3900
make run-mqtt           # MQTT Broker
make run-nats           # NATS (local)
make run-nats-cloud     # NATS (nube)
make run-all-containers # Inicia todos
```

### Instalar Dependencias

```bash
make install-deps
```

Descarga e instala todas las dependencias de Go del proyecto.

---

## 🛠️ Utilidades

### Formatear Código

```bash
make fmt
```

Formatea todo el código Go del proyecto usando `gofmt`.

### Análisis Estático

```bash
make vet
```

Ejecuta `go vet` para detectar errores comunes.

### Linting (Opcional)

```bash
make lint
```

Ejecuta `golangci-lint` si está instalado.

**Instalar golangci-lint**:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

## ℹ️ Información del Proyecto

```bash
make info
```

Muestra:
- Información del proyecto
- Versión de Go
- Estadísticas (archivos .go, tests, scripts)
- Estructura de módulos

---

## ⚡ Atajos Rápidos

Para comandos frecuentes, existen atajos de una sola letra:

| Atajo | Equivalente | Descripción |
|-------|-------------|-------------|
| `make t` | `make test` | Ejecutar tests |
| `make tc` | `make test-coverage` | Tests + cobertura |
| `make ta` | `make test-all` | Tests con script bash |
| `make c` | `make clean` | Limpiar artefactos |
| `make h` | `make help` | Mostrar ayuda |

---

## 🎯 Flujos de Trabajo Comunes

### Desarrollo Normal

```bash
# 1. Desarrollar código
# 2. Ejecutar tests
make test

# 3. Si los tests pasan, limpiar
make clean
```

### Antes de Commit

```bash
# 1. Formatear código
make fmt

# 2. Análisis estático
make vet

# 3. Ejecutar todos los tests con cobertura
make test-coverage

# 4. Ver reporte
make coverage-html
```

### Para la Tesis

```bash
# 1. Ejecutar todos los tests con script detallado
make test-all-coverage

# 2. Generar gráficos
make graficos

# 3. Revisar cobertura en HTML
make coverage-html
```

### Depuración de un Test Específico

```bash
# 1. Ejecutar solo el test que falla
make test-reglas-boolean

# 2. Si necesitas más detalle, usa go test directamente
cd test/edge/reglas
go test -v -run TestReglasBoolean
```

---

## 🔧 Personalización

### Modificar Timeouts

Edita la variable `TEST_TIMEOUT` en el Makefile:

```makefile
TEST_TIMEOUT := 30m  # Cambiar a 60m si necesitas más tiempo
```

### Agregar Nuevos Tests

1. Crea tu función de test en el archivo `*_test.go` correspondiente
2. El Makefile detectará automáticamente el nuevo test
3. Ejecuta con `make test` o `make test-coverage`

### Agregar Nuevo Target

Agrega al Makefile:

```makefile
## mi-comando: Descripción de mi comando
mi-comando:
	@echo "Ejecutando mi comando..."
	# ... comandos aquí
```

La línea con `##` hace que aparezca en `make help`.

---

## ❓ Troubleshooting

### Error: "make: comando no encontrado"

**Solución en Ubuntu/Debian**:
```bash
sudo apt-get install build-essential
```

**Solución en macOS**:
```bash
xcode-select --install
```

### Error: "No hay archivo de cobertura"

Si `make coverage-html` falla:

**Solución**:
```bash
# Primero genera el reporte
make test-coverage

# Luego ábrelo
make coverage-html
```

### Error: "bc: command not found"

Solo necesario si usas scripts bash con `-c`:

```bash
sudo apt-get install bc
```

Con el Makefile (`make test-coverage`) **NO** necesitas `bc`.

### Tests Fallan por Conflictos HTTP

Si ves errores como "address already in use":

**Solución 1**: Usa el Makefile (maneja conflictos automáticamente)
```bash
make test
```

**Solución 2**: Usa los scripts bash (tienen delays entre tests)
```bash
cd test/edge/reglas
./ejecutar_tests.sh
```

**Solución 3**: Mata procesos anteriores
```bash
pkill -f "go test"
```

---

## 📚 Recursos Adicionales

- **README.md**: Documentación general del proyecto
- **test/edge/reglas/README.md**: Documentación del motor de reglas
- **test/edge/GUIA_EJECUCION_TESTS.md**: Guía de ejecución de tests (bash)
- **docs/**: Otros documentos técnicos

---

## 💡 Tips y Mejores Prácticas

### 1. Usa `make test-coverage` Regularmente

Genera reportes HTML que son más fáciles de analizar que el output en terminal.

### 2. Tests Individuales para Depuración

En lugar de ejecutar toda la suite, ejecuta solo el test problemático:
```bash
make test-reglas-boolean
```

### 3. Limpia Entre Sesiones

```bash
make clean
```

Esto previene problemas con bases de datos temporales corruptas.

### 4. Revisa el Output Detallado

Si `make test` falla y quieres más detalle:
```bash
make test-all  # Usa script bash con output colorizado
```

### 5. Combina Comandos

```bash
make clean test-coverage coverage-html
```

Ejecuta múltiples targets en secuencia.

---

## 🎓 Conclusión

El Makefile simplifica enormemente el flujo de trabajo de testing en SensorWave. Los comandos más importantes son:

```bash
make help           # Cuando olvides un comando
make test           # Para desarrollo diario
make test-coverage  # Antes de commits importantes
make clean          # Después de cada sesión
```

Para cualquier duda, ejecuta `make help` o consulta esta guía.
