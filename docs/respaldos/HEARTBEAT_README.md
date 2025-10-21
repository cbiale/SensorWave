# Sistema de Heartbeats - Documentación

## Descripción General

Se ha implementado un sistema de heartbeats (latidos) que permite al despachador monitorear el estado de salud de los nodos edge en tiempo real a través de mensajería NATS.

## Arquitectura

```
┌─────────────┐                    ┌──────────────┐                    ┌─────────────┐
│   Nodo Edge │───► heartbeat ────►│     NATS     │───► heartbeat ────►│ Despachador │
│             │     (cada 30s)     │   Servidor   │                    │             │
└─────────────┘                    └──────────────┘                    └─────────────┘
       │                                                                      │
       │                                                                      │
       └──────────────────── suscripción inicial ──────────────────────────►│
       │                        (al iniciar)                                 │
       │                                                                      │
       └──────────────────── notificación nueva serie ────────────────────►│
                              (al crear serie)
```

## Componentes Implementados

### 1. En `edge/edge.go`

#### Struct `Heartbeat`
```go
type Heartbeat struct {
    NodeID    string    `json:"node_id"`
    Timestamp time.Time `json:"timestamp"`
    Activo    bool      `json:"activo"`
}
```

#### Función `enviarHeartbeat()`
- **Frecuencia**: Cada 30 segundos
- **Tópico NATS**: `despachador.heartbeat`
- **Contenido**: ID del nodo, timestamp actual, estado activo
- **Inicio automático**: Se lanza en goroutine al crear el nodo
- **Cierre limpio**: Se detiene cuando se cierra el nodo

**Características**:
- Se ejecuta en background (goroutine)
- Envía heartbeat periódicamente mientras el nodo está activo
- Se detiene cuando el nodo se cierra
- Maneja errores de serialización sin detener el proceso

### 2. En `despachador/despachador.go`

#### Struct `Heartbeat`
```go
type Heartbeat struct {
    NodeID    string    `json:"node_id"`
    Timestamp time.Time `json:"timestamp"`
    Activo    bool      `json:"activo"`
}
```

#### Función `escucharHeartbeats()`
- **Tópico NATS**: `despachador.heartbeat`
- **Acción**: Actualiza el timestamp `UltimoHeartbeat` y estado `Activo` del nodo
- **Persistencia**: Guarda el estado actualizado en PebbleDB

#### Función `monitorearNodosInactivos()`
- **Frecuencia de chequeo**: Cada 1 minuto
- **Timeout de inactividad**: 2 minutos
- **Acción**: Marca nodos como inactivos si no han enviado heartbeat en más de 2 minutos
- **Persistencia**: Actualiza el estado en PebbleDB

#### Función `ListarNodos()`
```go
func (m *ManagerDespachador) ListarNodos() map[string]*Nodo
```
- Retorna copia thread-safe de todos los nodos registrados
- Incluye información de estado, series, y último heartbeat

## Flujo de Comunicación

### 1. Registro Inicial del Nodo
```
1. Nodo Edge se crea con Crear(nombre, direccionNATS, puertoNATS)
2. Conecta a NATS
3. Envía suscripción inicial con:
   - ID del nodo
   - Series existentes (si las hay)
4. Despachador recibe y registra el nodo
5. Nodo inicia envío automático de heartbeats
```

### 2. Creación de Nueva Serie
```
1. Nodo Edge crea serie con CrearSerie()
2. Guarda serie en PebbleDB local
3. Envía notificación al despachador:
   - NodeID
   - Path de la serie
   - SerieID
4. Despachador actualiza registro del nodo
```

### 3. Monitoreo de Salud
```
Cada 30 segundos:
├─► Nodo Edge envía heartbeat
│   └─► Despachador actualiza UltimoHeartbeat
│
Cada 1 minuto:
└─► Despachador verifica nodos
    └─► Si (ahora - UltimoHeartbeat) > 2 minutos
        └─► Marca nodo como INACTIVO
```

## Configuración

### Parámetros de Tiempo

| Parámetro | Valor | Ubicación |
|-----------|-------|-----------|
| Intervalo de heartbeat | 30 segundos | `edge.go:enviarHeartbeat()` |
| Intervalo de monitoreo | 1 minuto | `despachador.go:monitorearNodosInactivos()` |
| Timeout de inactividad | 2 minutos | `despachador.go:monitorearNodosInactivos()` |

### Modificar Configuración

Para cambiar la frecuencia de heartbeats:
```go
// En edge.go:enviarHeartbeat()
ticker := time.NewTicker(30 * time.Second)  // Cambiar aquí
```

Para cambiar el timeout de inactividad:
```go
// En despachador.go:monitorearNodosInactivos()
if time.Since(nodo.UltimoHeartbeat) > 2*time.Minute {  // Cambiar aquí
```

## Ejemplo de Uso

```go
package main

import (
    "github.com/cbiale/sensorwave/despachador"
    "github.com/cbiale/sensorwave/edge"
)

func main() {
    // 1. Crear despachador
    desp, _ := despachador.CrearDespachador("desp.db", "localhost", "4222")
    defer desp.Cerrar()

    // 2. Crear nodo edge (automáticamente se registra y envía heartbeats)
    nodo, _ := edge.Crear("nodo.db", "localhost", "4222")
    defer nodo.Cerrar()

    // 3. Verificar estado de nodos
    nodos := desp.ListarNodos()
    for id, nodo := range nodos {
        fmt.Printf("Nodo: %s, Activo: %v, Último heartbeat: %s\n",
            id, nodo.Activo, nodo.UltimoHeartbeat)
    }
}
```

## Testing

Un programa de demostración está disponible en `test/despachador/main.go` que:

1. Crea un despachador
2. Crea dos nodos edge
3. Monitorea heartbeats durante 90 segundos
4. Simula la caída de un nodo
5. Espera a que se detecte como inactivo
6. Muestra el estado final

Para ejecutar:
```bash
# Asegurar que NATS esté ejecutándose
docker run -p 4222:4222 nats:latest

# Ejecutar la demostración
cd test/despachador
go run main.go
```

## Logs

El sistema genera logs informativos en cada evento:

### En el Nodo Edge:
```
2025/10/21 10:30:00 Suscripción informada: nodo edge-hostname-abc123 con 2 series
2025/10/21 10:30:00 Heartbeat enviado desde nodo edge-hostname-abc123
2025/10/21 10:30:30 Heartbeat enviado desde nodo edge-hostname-abc123
2025/10/21 10:31:00 Nueva serie informada: dispositivo_001/temp (ID: 1) en nodo edge-hostname-abc123
```

### En el Despachador:
```
2025/10/21 10:30:00 Nodo suscrito: edge-hostname-abc123 con 2 series
2025/10/21 10:30:00 Heartbeat recibido de nodo edge-hostname-abc123
2025/10/21 10:30:30 Heartbeat recibido de nodo edge-hostname-abc123
2025/10/21 10:32:30 Nodo edge-hostname-abc123 marcado como INACTIVO (último heartbeat: 2025-10-21T10:30:30Z)
```

## Ventajas del Sistema

1. **Detección automática de fallos**: Identifica nodos caídos sin intervención manual
2. **Estado en tiempo real**: El despachador siempre conoce qué nodos están disponibles
3. **Persistencia**: El estado se guarda en PebbleDB
4. **Escalable**: Soporta múltiples nodos sin problemas de rendimiento
5. **Desacoplado**: Usa NATS para comunicación asíncrona
6. **Thread-safe**: Operaciones concurrentes protegidas con mutexes

## Limitaciones y Consideraciones

1. **Requiere NATS**: El servidor NATS debe estar ejecutándose
2. **No detecta particiones de red**: Un nodo puede estar vivo pero inaccesible
3. **Overhead de red**: Heartbeats cada 30s generan tráfico constante
4. **Latencia de detección**: Hasta 2 minutos para detectar un nodo caído

## Mejoras Futuras Sugeridas

1. **Métricas en heartbeat**: Agregar uso de CPU, memoria, tamaño de DB
2. **Configuración dinámica**: Permitir ajustar intervalos sin recompilar
3. **Alertas**: Notificaciones cuando un nodo se marca como inactivo
4. **Reconexión automática**: Reintentar conectar a nodos inactivos
5. **Dashboard web**: Visualización en tiempo real del estado del cluster
