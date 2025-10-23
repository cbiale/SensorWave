# Cambios en Despachador: Eliminación de Pebble (Modo Stateless)

## Resumen

Se modificó `despachador/despachador.go` para eliminar la dependencia de PebbleDB, convirtiendo el despachador en un servicio **stateless** que mantiene el estado solo en memoria.

## Cambios Realizados

### 1. Eliminación de imports
- ❌ Removido: `"github.com/cockroachdb/pebble"`

### 2. Modificación del struct `ManagerDespachador`

**ANTES:**
```go
type ManagerDespachador struct {
    db      *pebble.DB
    cliente *sw_cliente.ClienteNATS
    nodos   map[string]*Nodo
    mu      sync.RWMutex
    done    chan struct{}
}
```

**DESPUÉS:**
```go
type ManagerDespachador struct {
    cliente *sw_cliente.ClienteNATS
    nodos   map[string]*Nodo  // Solo en memoria
    mu      sync.RWMutex
    done    chan struct{}
}
```

### 3. Simplificación de `CrearDespachador()`

**Eliminado:**
- ❌ Apertura de base de datos Pebble
- ❌ Llamada a `cargarNodos()` (ya no existe)
- ❌ Manejo de errores de Pebble

**Resultado:**
- ✅ Código más simple y directo
- ✅ Sin dependencias de disco
- ✅ Inicio instantáneo
- ✅ Mensaje actualizado: "Despachador iniciado en modo stateless (solo memoria)"

### 4. Eliminación de `cargarNodos()`

**Razón:** Ya no hay persistencia entre reinicios. El estado se reconstruye automáticamente mediante:
- Suscripciones de nodos edge
- Heartbeats (cada 30 segundos)
- Notificaciones de nuevas series

### 5. Simplificación de `Cerrar()`

**ANTES:**
```go
func (m *ManagerDespachador) Cerrar() error {
    close(m.done)
    if m.cliente != nil {
        m.cliente.Desconectar()
    }
    if m.db != nil {
        return m.db.Close()
    }
    return nil
}
```

**DESPUÉS:**
```go
func (m *ManagerDespachador) Cerrar() error {
    close(m.done)
    if m.cliente != nil {
        m.cliente.Desconectar()
    }
    return nil
}
```

### 6. Eliminación de escrituras a disco

En todos los handlers de eventos, se eliminaron las llamadas a `m.db.Set()`:

- `escucharSuscripciones()` - línea 148
- `escucharNuevasSeries()` - línea 186  
- `escucharHeartbeats()` - línea 222
- `monitorearNodosInactivos()` - línea 250

**Ahora solo se actualiza el mapa en memoria:**
```go
m.mu.Lock()
m.nodos[nodeID] = nodo  // Solo memoria
m.mu.Unlock()
```

## Beneficios

### ✅ Simplicidad
- Menos código (eliminadas ~30 líneas)
- Sin gestión de archivos de base de datos
- Sin manejo de errores de I/O a disco

### ✅ Performance
- Sin latencia de escritura a disco
- Operaciones en nanosegundos vs milisegundos
- Sin flush de disco (pebble.Sync)

### ✅ Escalabilidad Horizontal
- **Stateless** → fácil de escalar
- Sin estado local divergente
- Ideal para contenedores/Kubernetes
- Auto-scaling sin complicaciones

### ✅ Costos en la Nube
- **$0 en almacenamiento** para despachadores
- Sin volúmenes EBS/Persistent Disk
- Sin IOPS adicionales
- Compatible con arquitecturas serverless

### ✅ Alta Disponibilidad
- Múltiples despachadores reciben los mismos mensajes NATS
- Cada uno mantiene vista completa de nodos edge
- Si un despachador cae, otros continúan sin interrupción
- Despachador nuevo se sincroniza en <30 segundos (próximo heartbeat)

## Trade-offs Aceptables

### ⚠️ Sin persistencia entre reinicios
- **Impacto:** Despachador reiniciado arranca con estado vacío
- **Mitigación:** Se reconstruye automáticamente en <30 segundos vía heartbeats
- **Evaluación:** Aceptable para arquitectura distribuida con múltiples despachadores

### ⚠️ Sin historial de nodos inactivos
- **Impacto:** No se recuerdan nodos que dejaron de estar activos
- **Mitigación:** No requerido - los datos reales están en nodos edge
- **Evaluación:** Suficiente para casos de uso actuales

## Compatibilidad

### ✅ Interfaz pública sin cambios
- `CrearDespachador()` - misma firma
- `ListarNodos()` - misma firma
- `Cerrar()` - misma firma

### ✅ Funcionamiento con nodos edge
- Nodos edge publican igual a NATS
- Recepción de suscripciones: ✅
- Recepción de nuevas series: ✅
- Recepción de heartbeats: ✅
- Monitoreo de inactividad: ✅

### ✅ Tests
- Test de integración (`test/edge/integrador/`) funciona correctamente
- Nodo edge funciona en modo autónomo (sin NATS) para testing
- Sin cambios requeridos en tests existentes

## Arquitectura Recomendada para Producción

```
┌─────────────────────────┐
│   Load Balancer (ALB)   │
└────────────┬────────────┘
             │
    ┌────────┼────────┐
    │        │        │
┌───▼──┐ ┌──▼───┐ ┌──▼───┐
│Desp 1│ │Desp 2│ │Desp N│  Stateless
│(mem) │ │(mem) │ │(mem) │  Auto-scaling
└───┬──┘ └──┬───┘ └──┬───┘
    │       │        │
    └───────┼────────┘
            │
    ┌───────▼────────┐
    │  NATS Cluster  │
    └───────┬────────┘
            │
    ┌───────┴────────┐
    │  Nodos Edge    │
    │  (con Pebble)  │
    └────────────────┘
```

## Próximos Pasos (Opcional)

Si se requiere estado compartido persistente:

1. **NATS KV Store (JetStream)**
   - Agregar campo `kv nats.KeyValue` al struct
   - Reemplazar `m.nodos` por lecturas/escrituras a KV
   - Requiere habilitar JetStream en NATS

2. **Queue Groups**
   - Modificar suscripciones para distribuir carga
   - Solo si se necesita escalado de procesamiento (no solo HA)
   - Requiere modificar `ClienteNATS` para soportar queue subscriptions

3. **Servicio de descubrimiento**
   - Si cliente no puede broadcast a todos los despachadores
   - Implementar routing basado en node_id
   - Solo necesario para optimización avanzada

## Conclusión

✅ **Cambios completados exitosamente**
- Despachador ahora es stateless
- Código más simple y mantenible
- Listo para escalado horizontal
- Compatible con arquitecturas cloud-native
- Tests funcionando correctamente
