# Comparativa: CoAP vs HTTP - Implementación Actual

## Análisis de `servidor_coap.go` y `servidor_http.go`

---

## 1. GET (Suscripción)

| Aspecto | CoAP | HTTP |
|---------|------|------|
| **Método** | `GET` + `Observe: 0` | `GET` |
| **Línea de código** | L63-64 | L34-35 |
| **Obtención del topic** | `r.Path()` → Del path URL | `r.URL.Query().Get("topico")` → Query param |
| **Ejemplo de request** | `GET coap://server/sensor/001/temp` + Observe:0 | `GET http://server/sensorwave?topico=sensor/001/temp` |
| **Ubicación del topic** | **En el PATH** (`/sensor/001/temp`) | **En QUERY PARAM** (`?topico=...`) |
| **Validación** | No valida topic vacío | Valida topic vacío (L49-51) |
| **Almacenamiento** | `observadores[topico]` → array de Conexion | `clientesPorTopico[topico]` → array de Cliente |
| **Estructura guardada** | `Conexion{conexion, context, token}` | `Cliente{Channel}` |
| **Tipo de conexión** | Observador CoAP (push desde servidor) | SSE - Server-Sent Events (stream) |
| **Encabezados especiales** | Opción `Observe` en CoAP | `Content-Type: text/event-stream` |
| **Mantiene conexión** | ✅ Sí (observador activo) | ✅ Sí (SSE mantiene conexión abierta) |
| **Desconexión automática** | Cliente envía `Observe: 1` | Cliente cierra conexión HTTP |

---

## 2. POST (Publicación)

| Aspecto | CoAP | HTTP |
|---------|------|------|
| **Método** | `POST` | `POST` |
| **Línea de código** | L69-84 | L37-39, L103-129 |
| **Obtención del topic** | **DUAL**: `r.Path()` + `mensaje.Topico` (body) | `mensaje.Topico` (solo del body JSON) |
| **Ejemplo de request** | `POST coap://server/sensor/001/temp`<br>Body: `{"topico": "...", "dato": 23.5}` | `POST http://server/sensorwave`<br>Body: `{"topico": "sensor/001/temp", "dato": 23.5}` |
| **Path usado** | ✅ Sí - Se pasa a `manejarPublicacionCoAP()` (L84) | ❌ No - Path fijo `/sensorwave` |
| **Body JSON** | ✅ Requerido - parseado en L72-82 | ✅ Requerido - parseado en L105-110 |
| **Validación del topic** | Valida que `mensaje.Topico` exista (implícito) | Valida explícitamente (L112-115) |
| **Parámetros pasados** | `manejarPublicacionCoAP(w, r, ruta, mensaje)` | Solo `mensaje` (topic en body) |
| **¿Usa el path?** | **SÍ** - pero también usa `mensaje.Topico` | **NO** - solo usa `mensaje.Topico` del body |
| **Respuesta** | `codes.Created` (2.01) | `http.StatusOK` (200) |
| **Distribución** | Llama `enviarXXX()` con `mensaje.Topico` | Llama `enviarXXX()` con `mensaje.Topico` |

---

## 3. DELETE (Desuscripción)

| Aspecto | CoAP | HTTP |
|---------|------|------|
| **Método** | `GET` + `Observe: 1` | `DELETE` |
| **Línea de código** | L66-67, L130-148 | L40-42, L135-137 |
| **Implementación** | ✅ Funcional | ❌ **NO IMPLEMENTADA** (función vacía) |
| **Obtención del topic** | `r.Path()` | No implementado |
| **Acción** | Remueve observador del map | N/A |
| **Validación token** | Sí - compara `r.Token()` con tokens almacenados | N/A |

---

## 4. Diferencias Clave

### 4.1 Ubicación del Topic

| Protocolo | Suscripción (GET) | Publicación (POST) |
|-----------|-------------------|-------------------|
| **CoAP** | Path URL | **Path URL + Body JSON** (redundante) |
| **HTTP** | Query param | Body JSON (único) |

### 4.2 Problema en CoAP POST

**CoAP recibe el topic en DOS lugares:**

```go
// Línea 48: topic del path
ruta, err := r.Path()  // ej: "/sensor/001/temp"

// Línea 78-82: topic del body JSON
json.Unmarshal(cuerpo, &mensaje)  // mensaje.Topico = "sensor/001/temp"

// Línea 84: pasa AMBOS
manejarPublicacionCoAP(w, r, ruta, mensaje)
```

**Pero en las funciones `enviarXXX()` solo se usa `mensaje.Topico`, no `ruta`.**

### 4.3 Consistencia en HTTP

HTTP usa **solo una fuente** del topic:
- **GET**: Query param (`?topico=...`)
- **POST**: Body JSON (`{"topico": "..."}`)

---

## 5. Estructura de Datos

### CoAP
```go
type Conexion struct {
    conexion mux.Conn       // Conexión CoAP
    context  context.Context // Contexto
    token    []byte          // Token único del observador
}

// Mapa: topic → array de conexiones
observadores = map[string][]Conexion
```

### HTTP
```go
type Cliente struct {
    Channel chan string  // Canal para SSE
}

// Mapa: topic → array de clientes
clientesPorTopico = map[string][]*Cliente
```

---

## 6. Flujo de Mensajes

### Suscripción y Notificación

#### CoAP:
1. Cliente: `GET /sensor/001/temp` + `Observe: 0`
2. Servidor: Guarda en `observadores["/sensor/001/temp"]`
3. Cuando llega POST con ese topic → busca observadores y envía con `enviarRespuesta()`

#### HTTP:
1. Cliente: `GET /sensorwave?topico=sensor/001/temp`
2. Servidor: Guarda en `clientesPorTopico["sensor/001/temp"]`
3. Cuando llega POST con ese topic → busca clientes y escribe en `cliente.Channel`

---

## 7. Problemas Identificados

### 7.1 CoAP - Redundancia en POST
- ❌ Topic viene en **path** y en **body**
- ❌ No queda claro cuál es la fuente de verdad
- ❌ `ruta` se pasa pero no se usa en distribución

### 7.2 CoAP - Wildcards en Path
- ❌ No soporta wildcards (`#`, `*`) en el path por limitaciones de URL
- ⚠️ Caracteres especiales requieren encoding

### 7.3 HTTP - DELETE no implementado
- ❌ Función `manejarDesuscripcionHTTP()` está vacía
- ⚠️ Clientes solo se desuscriben cerrando la conexión

### 7.4 Ambos - No validan wildcards
- ❌ No hay validación de formato de wildcards
- ❌ No hay conversión entre formatos (MQTT ↔ NATS)

---

## 8. Recomendaciones

### Para CoAP:
1. **Eliminar redundancia en POST**: 
   - Opción A: Topic solo en body JSON (recomendado)
   - Opción B: Topic solo en query param (consistente con HTTP GET)

2. **Suscripción con wildcards**:
   - Usar query param: `GET /subscribe?topic=sensor/%23`
   - Evita problemas con caracteres especiales en path

### Para HTTP:
1. **Implementar DELETE** para desuscripción explícita
2. **Validar wildcards** en GET (solo permitir en suscripción)

### Para Ambos:
1. **Agregar validación de wildcards**:
   - `#` solo al final
   - `*` en cualquier nivel
2. **Normalizar topics** a formato interno
3. **No permitir wildcards en POST** (publicación)

---

## 9. Tabla Resumen

| Característica | CoAP | HTTP | ¿Consistente? |
|----------------|------|------|---------------|
| GET obtiene topic de | Path | Query param | ❌ Diferente |
| POST obtiene topic de | Path + Body | Body | ⚠️ CoAP redundante |
| Suscripción | Observe | SSE | ✅ Ambos persistentes |
| Desuscripción | GET + Observe:1 | DELETE (vacío) | ⚠️ HTTP sin impl. |
| Validación topic vacío | No | Sí | ❌ Inconsistente |
| Soporte wildcards | No | No | ✅ Ambos faltan |
| Topic en distribución | `mensaje.Topico` | `mensaje.Topico` | ✅ Consistente |

---

## 10. Conclusiones

1. **HTTP es más consistente** en el uso del topic (query param para GET, body para POST)
2. **CoAP tiene redundancia** en POST (topic en path y body)
3. **Ninguno soporta wildcards** actualmente
4. **La distribución interna es consistente** (ambos usan `mensaje.Topico`)
5. **Se requiere refactoring** para:
   - Eliminar redundancia en CoAP POST
   - Agregar soporte de wildcards en suscripciones
   - Implementar DELETE en HTTP
   - Validar y normalizar topics

