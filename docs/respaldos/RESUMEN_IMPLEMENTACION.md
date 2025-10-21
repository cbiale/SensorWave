# Resumen de Implementación: Sistema de Heartbeats y Comunicación NATS

## Cambios Realizados

### 1. Modificaciones en `edge/edge.go`

**Nuevas importaciones:**
- `encoding/json` - Para serialización de mensajes
- `github.com/cbiale/sensorwave/middleware/cliente_nats` - Cliente NATS

**Nuevos structs:**
```go
type SuscripcionNodo struct {
    ID     string            `json:"id"`
    Series map[string]string `json:"series"`
}

type NuevaSerie struct {
    NodeID  string `json:"node_id"`
    Path    string `json:"path"`
    SerieID int    `json:"serie_id"`
}

type Heartbeat struct {
    NodeID    string    `json:"node_id"`
    Timestamp time.Time `json:"timestamp"`
    Activo    bool      `json:"activo"`
}
```

**Campo agregado a ManagerEdge:**
```go
cliente sw_cliente.Cliente // Cliente NATS
```

**Función Crear() modificada:**
- Firma: `func Crear(nombre string, direccionNATS string, puertoNATS string)`
- Conecta a servidor NATS
- Informa suscripción inicial al despachador
- Inicia goroutine de heartbeats

**Nuevas funciones:**
- `informarSuscripcion()` - Envía ID y series al despachador
- `informarNuevaSerie()` - Notifica nueva serie al despachador  
- `enviarHeartbeat()` - Envía heartbeat cada 30 segundos

**Cerrar() modificado:**
- Desconecta cliente NATS antes de cerrar

**CrearSerie() modificado:**
- Informa nueva serie al despachador vía NATS

### 2. Modificaciones en `despachador/despachador.go`

**Nuevos structs:**
```go
type Heartbeat struct {
    NodeID    string    `json:"node_id"`
    Timestamp time.Time `json:"timestamp"`
    Activo    bool      `json:"activo"`
}
```

**Campos agregados a ManagerDespachador:**
```go
mu   sync.RWMutex
done chan struct{}
```

**CrearDespachador() modificado:**
- Firma: `func CrearDespachador(nombre string, direccionNATS string, puertoNATS string)`
- Inicia 4 goroutines:
  - `escucharSuscripciones()` - Procesa registros de nodos
  - `escucharNuevasSeries()` - Procesa notificaciones de series
  - `escucharHeartbeats()` - Procesa heartbeats
  - `monitorearNodosInactivos()` - Detecta nodos caídos

**Nuevas funciones:**
- `escucharSuscripciones()` - Escucha `despachador.suscripcion`
- `escucharNuevasSeries()` - Escucha `despachador.nueva_serie`
- `escucharHeartbeats()` - Escucha `despachador.heartbeat`
- `monitorearNodosInactivos()` - Chequea cada minuto si nodos están activos
- `ListarNodos()` - Retorna map de nodos registrados

**Cerrar() modificado:**
- Cierra canal `done` para detener goroutines

### 3. Nuevo archivo de prueba

**`test/despachador/main.go`:**
- Demuestra el sistema completo
- Crea despachador y 2 nodos edge
- Crea series e inserta datos
- Monitorea heartbeats durante 90 segundos
- Simula falla de nodo
- Espera detección de inactividad

### 4. Documentación

**`HEARTBEAT_README.md`:**
- Descripción completa del sistema
- Arquitectura y diagramas
- Configuración de parámetros
- Ejemplos de uso
- Logs y troubleshooting

## Tópicos NATS Utilizados

1. **`despachador.suscripcion`**: Edge → Despachador (suscripción inicial)
2. **`despachador.nueva_serie`**: Edge → Despachador (notificación de serie)
3. **`despachador.heartbeat`**: Edge → Despachador (heartbeat periódico)

## Parámetros Configurables

| Parámetro | Valor Actual | Ubicación |
|-----------|-------------|-----------|
| Intervalo heartbeat | 30 segundos | edge.go:enviarHeartbeat() |
| Intervalo monitoreo | 1 minuto | despachador.go:monitorearNodosInactivos() |
| Timeout inactividad | 2 minutos | despachador.go:monitorearNodosInactivos() |

## Estado de Compilación

✅ `edge/edge.go` - Compila correctamente
✅ `despachador/despachador.go` - Compila correctamente  
✅ `test/despachador/main.go` - Compila correctamente

## Próximos Pasos Sugeridos

1. **Actualizar tests existentes** en `test/edge/` para usar nueva firma de Crear()
2. **Implementar manejo de errores** más robusto en comunicación NATS
3. **Agregar métricas** (CPU, memoria, disco) en heartbeats
4. **Crear dashboard web** para visualizar estado del cluster
5. **Implementar reconexión automática** cuando NATS falla

## Comandos de Verificación

```bash
# Compilar edge
go build ./edge/

# Compilar despachador
go build -o /tmp/desp ./despachador/

# Compilar test
go build -o /tmp/test_desp ./test/despachador/

# Ejecutar test (requiere NATS en localhost:4222)
docker run -d -p 4222:4222 nats:latest
/tmp/test_desp
```

## Cambios Necesarios en Código Existente

Los siguientes archivos necesitan actualizar la llamada a `edge.Crear()`:

- `test/edge/path_tags_ejemplo/main.go`
- `test/edge/reglas/main.go`
- `test/edge/integrador/main.go`
- `test/edge/series/main.go`

Cambiar de:
```go
manager, err := edge.Crear("nombre.db")
```

A:
```go
manager, err := edge.Crear("nombre.db", "localhost", "4222")
```
