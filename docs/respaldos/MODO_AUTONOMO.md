# Modo Autónomo: Operación Sin Conexión NATS

## Descripción

El nodo edge está diseñado para operar en **modo autónomo** cuando no hay conexión disponible con el servidor NATS. Esto es crítico para sistemas IoT que deben funcionar de forma independiente, especialmente en ubicaciones remotas o con conectividad intermitente.

## Comportamiento

### Nodo Edge (`edge.go`)

#### ✅ CON conexión NATS:
```
[INICIO]
  ↓
Conectar a PebbleDB ✓
  ↓
Conectar a NATS ✓
  ↓
Informar suscripción al despachador ✓
  ↓
Iniciar heartbeats (cada 30s) ✓
  ↓
[OPERACIÓN NORMAL CON CLUSTER]
  - Almacena datos localmente
  - Notifica nuevas series al despachador
  - Envía heartbeats periódicos
  - Participa en el cluster distribuido
```

#### ⚠️ SIN conexión NATS:
```
[INICIO]
  ↓
Conectar a PebbleDB ✓
  ↓
Intentar conectar a NATS ✗
  ↓
Log: "ADVERTENCIA: Nodo edge funcionando en modo autónomo sin NATS"
  ↓
[OPERACIÓN AUTÓNOMA LOCAL]
  - Almacena datos localmente ✓
  - NO notifica al despachador
  - NO envía heartbeats
  - Funciona de forma independiente
```

### Despachador (`despachador.go`)

El despachador **REQUIERE** conexión NATS para funcionar, ya que su propósito es coordinar el cluster:

#### ✅ CON conexión NATS:
```
Despachador inicia correctamente
Escucha suscripciones de nodos
Monitorea heartbeats
```

#### ❌ SIN conexión NATS:
```
ERROR: "despachador requiere conexión NATS"
No inicia
```

## Funcionalidades por Modo

| Funcionalidad | Con NATS | Sin NATS |
|--------------|----------|----------|
| **Crear series** | ✓ | ✓ |
| **Insertar datos** | ✓ | ✓ |
| **Consultar datos** | ✓ | ✓ |
| **Comprimir bloques** | ✓ | ✓ |
| **Almacenar en PebbleDB** | ✓ | ✓ |
| **Motor de reglas** | ✓ | ✓ |
| **Notificar al despachador** | ✓ | ✗ |
| **Enviar heartbeats** | ✓ | ✗ |
| **Participar en cluster** | ✓ | ✗ |

## Logs de Diagnóstico

### Modo con NATS exitoso:
```
2025/10/21 10:00:00 NodeID cargado desde DB: edge-hostname-abc123
2025/10/21 10:00:00 Conectado exitosamente a NATS en nats://localhost:4222
2025/10/21 10:00:00 Suscripción informada: nodo edge-hostname-abc123 con 2 series
2025/10/21 10:00:00 Nodo edge conectado al cluster vía NATS
2025/10/21 10:00:30 Heartbeat enviado desde nodo edge-hostname-abc123
```

### Modo autónomo (sin NATS):
```
2025/10/21 10:00:00 NodeID cargado desde DB: edge-hostname-abc123
2025/10/21 10:00:00 Advertencia: No se pudo conectar a NATS en nats://localhost:4222: dial tcp [::1]:4222: connect: connection refused
2025/10/21 10:00:00 ADVERTENCIA: Nodo edge funcionando en modo autónomo sin NATS: dial tcp [::1]:4222: connect: connection refused
2025/10/21 10:00:00 El nodo continuará operando localmente. Las funciones de cluster están deshabilitadas.
```

## Escenarios de Uso

### 1. **Despliegue Inicial Sin Infraestructura**
```go
// El nodo puede iniciar sin NATS disponible
nodo, err := edge.Crear("sensor1.db", "localhost", "4222")
// err == nil, nodo funciona en modo autónomo
```

### 2. **Pérdida de Conexión Durante Operación**
```go
// El nodo ya está corriendo con NATS
// La red falla → NATS se desconecta

// Comportamiento:
// - Datos continúan almacenándose localmente
// - Heartbeats/notificaciones fallan silenciosamente
// - No afecta operación local del nodo
```

### 3. **Reconexión al Cluster**
```
Actualmente: NO implementado
Futuro: Reintentar conexión periódicamente
        Sincronizar estado al reconectar
```

## Protecciones Implementadas

### En `cliente_nats.go`:

```go
func Conectar(direccion string, puerto string) (*ClienteNATS, error) {
    // Retorna error en vez de panic con log.Fatalf()
    if err != nil {
        return nil, err  // ✓ Permite manejo del error
    }
}

func (c *ClienteNATS) Publicar(topico string, mensaje interface{}) {
    if c == nil || c.conn == nil {
        return  // ✓ No falla si no hay conexión
    }
    // ... publicar
}
```

### En `edge.go`:

```go
func (me *ManagerEdge) informarSuscripcion() error {
    if me.cliente == nil {
        return nil  // ✓ No intenta enviar si no hay cliente
    }
    // ... enviar suscripción
}

func (me *ManagerEdge) enviarHeartbeat() {
    if me.cliente == nil {
        return  // ✓ Heartbeat se detiene si no hay cliente
    }
    // ... enviar heartbeats
}
```

## Ventajas del Diseño

1. **Alta disponibilidad local**: El nodo edge nunca se detiene por falta de NATS
2. **Operación offline**: Perfecto para IoT en ubicaciones remotas
3. **Resiliencia**: Continúa almacenando datos durante interrupciones de red
4. **Deployment flexible**: No requiere infraestructura completa para iniciar
5. **Degradación elegante**: Funcionalidades se degradan gradualmente, no fallan catastróficamente

## Limitaciones

1. **Sin coordinación cluster**: El despachador no conoce el estado del nodo
2. **Sin notificaciones**: Nuevas series no se propagan al cluster
3. **Sin heartbeats**: El despachador marcará el nodo como inactivo después de 2 minutos
4. **No hay reconexión automática**: Actualmente el nodo no reintenta conectar

## Mejoras Futuras Recomendadas

### 1. Reconexión Automática
```go
func (me *ManagerEdge) intentarReconexion() {
    ticker := time.NewTicker(5 * time.Minute)
    for {
        select {
        case <-ticker.C:
            if me.cliente == nil {
                cliente, err := sw_cliente.Conectar(me.natsAddr, me.natsPort)
                if err == nil {
                    me.cliente = cliente
                    me.informarSuscripcion()
                    go me.enviarHeartbeat()
                    log.Printf("Reconectado exitosamente a NATS")
                }
            }
        }
    }
}
```

### 2. Cola de Mensajes Offline
```go
// Almacenar mensajes mientras esté offline
// Enviar cuando se reconecte
type MensajePendiente struct {
    Topico  string
    Payload []byte
    Timestamp time.Time
}
```

### 3. Sincronización al Reconectar
```go
// Enviar todas las series creadas durante offline
// Actualizar el despachador con el estado actual
```

## Ejemplo de Código

### Creación tolerante a fallos:
```go
package main

import (
    "log"
    "github.com/cbiale/sensorwave/edge"
)

func main() {
    // Intenta conectar a NATS pero no falla si no está disponible
    nodo, err := edge.Crear("sensor.db", "localhost", "4222")
    if err != nil {
        log.Fatalf("Error crítico al crear nodo: %v", err)
    }
    defer nodo.Cerrar()
    
    // El nodo funciona independientemente de si NATS está disponible
    // Si NATS está disponible: modo cluster
    // Si NATS no está disponible: modo autónomo
    
    // Operaciones normales funcionan en ambos modos
    serie := edge.Serie{
        Path: "temp/sensor1",
        TipoDatos: edge.TipoNumerico,
        TamañoBloque: 100,
    }
    
    nodo.CrearSerie(serie)  // ✓ Funciona siempre
    nodo.Insertar("temp/sensor1", time.Now().UnixNano(), 25.5)  // ✓ Funciona siempre
}
```

## Testing

### Test sin NATS:
```bash
# No iniciar NATS
# go run main.go
# Resultado: Nodo funciona en modo autónomo
```

### Test con NATS:
```bash
# Iniciar NATS
docker run -p 4222:4222 nats:latest

# go run main.go
# Resultado: Nodo se conecta al cluster
```

### Test de pérdida de conexión:
```bash
# 1. Iniciar con NATS
# 2. Detener NATS mientras corre
# 3. Observar que continúa funcionando localmente
```

## Conclusión

El sistema está diseñado para **priorizar la disponibilidad local** sobre la coordinación del cluster. Un nodo edge puede operar completamente de forma independiente, lo que lo hace ideal para despliegues en ubicaciones remotas o con conectividad inestable.
