# Análisis de Funcionalidad del Sistema de Protocolos IoT

## Resumen Ejecutivo

El sistema implementado en `coap.c`, `mqtt.c` y `http.c` **ES COMPLETAMENTE FUNCIONAL** para todas las operaciones de conectividad IoT requeridas: conexión, suscripción a tópicos, desuscripción de tópicos, publicación en tópicos y desconexión. Cada protocolo implementa las mismas funcionalidades con sus características específicas.

## Funcionalidades Implementadas

### ✅ CONEXIÓN/DESCONEXIÓN

#### CoAP (coap.c:210-316, 545-577)
- **`coap_conectar()`**: 
  - Inicializa contexto CoAP
  - Resuelve dirección del servidor usando `getaddrinfo()`
  - Crea sesión UDP con el servidor
  - Registra handlers de respuesta
  - Crea tarea dedicada para procesamiento de mensajes
  
- **`coap_desconectar()`**: 
  - Cancela todas las observaciones activas
  - Libera sesión y contexto CoAP
  - Limpia recursos y memoria
  - Manejo thread-safe con mutex

#### MQTT (mqtt.c:176-226, 373-394)
- **`mqtt_conectar()`**: 
  - Configura cliente MQTT con URI del broker
  - Genera ClientID único usando `esp_random()`
  - Registra event handler para eventos de conexión
  - Inicializa lista de suscripciones
  
- **`mqtt_desconectar()`**: 
  - Detiene y destruye cliente MQTT
  - Libera lista de suscripciones
  - Limpia recursos de memoria
  - Actualiza estado de conexión thread-safe

#### HTTP (http.c:167-202, 383-409)
- **`http_conectar()`**: 
  - Inicializa cliente HTTP ESP-IDF
  - Construye URL base (`http://host:puerto`)
  - Configura event handler para eventos HTTP
  - Inicializa lista de suscripciones
  
- **`http_desconectar()`**: 
  - Detiene todas las tareas SSE activas
  - Libera lista de suscripciones
  - Limpia cliente HTTP y recursos
  - Manejo thread-safe con mutex

### ✅ SUSCRIPCIÓN/DESUSCRIPCIÓN

#### CoAP (coap.c:319-415, 476-542)
- **`coap_suscribir()`**: 
  - Implementa patrón Observer usando PDU GET con opción OBSERVE
  - Soporte para URI paths multinivel con `coap_agregar_uri_path()`
  - Gestiona tokens únicos para cada observación
  - Lista enlazada thread-safe para observaciones activas
  
- **`coap_desuscribir()`**: 
  - Cancela observación enviando OBSERVE=1
  - Limpia tokens y libera memoria
  - Remueve de lista de observaciones de forma segura

#### MQTT (mqtt.c:229-291, 331-370)
- **`mqtt_suscribir()`**: 
  - Usa `esp_mqtt_client_subscribe()` del ESP-IDF
  - Gestiona callbacks por tópico en lista enlazada
  - Actualiza callbacks existentes sin re-suscripción
  - Thread-safe con mutex para acceso a lista
  
- **`mqtt_desuscribir()`**: 
  - Usa `esp_mqtt_client_unsubscribe()` del ESP-IDF
  - Limpia entrada de lista interna
  - Libera memoria de tópico y estructura

#### HTTP (http.c:205-268, 320-380)
- **`http_suscribir()`**: 
  - Crea tarea FreeRTOS dedicada para conexión SSE
  - Configura headers SSE: `Accept: text/event-stream`, `Cache-Control: no-cache`
  - Mantiene conexión persistente por tópico
  - Parsea eventos SSE y ejecuta callbacks
  
- **`http_desuscribir()`**: 
  - Detiene tarea SSE específica del tópico
  - Envía HTTP DELETE al servidor para notificar desuscripción
  - Limpia entrada de lista y libera memoria
  - Thread-safe con mutex

### ✅ PUBLICACIÓN

#### CoAP (coap.c:418-473)
- **`coap_publicar()`**: 
  - Usa PDU POST con Content-Format text/plain
  - Crea mensajes JSON compatibles con Go
  - Encoding base64 para payload usando mbedTLS
  - Soporte para URI paths multinivel
  - Verificación de estado de conexión

#### MQTT (mqtt.c:294-328)
- **`mqtt_publicar()`**: 
  - Usa `esp_mqtt_client_publish()` con QoS 0
  - Crea mensajes JSON compatibles con Go
  - Payload en texto plano (sin encoding)
  - Verificación de estado de conexión
  - Manejo detallado de errores

#### HTTP (http.c:271-317)
- **`http_publicar()`**: 
  - Usa HTTP POST a endpoint `/sensorwave`
  - Headers: `Content-Type: application/json`
  - Crea mensajes JSON compatibles con Go
  - Verificación de código de estado HTTP (200 = éxito)
  - Manejo detallado de errores con logging

## Características del Sistema

### Características Comunes
- **Thread-safety**: Uso de mutexes para acceso concurrente seguro
- **Gestión de estado**: Variables `*_conectado` protegidas por mutex
- **Formato JSON**: Mensajes compatibles con sistema Go del backend
- **Manejo de errores**: Logging detallado con ESP_LOG para debugging
- **Gestión de memoria**: Liberación adecuada de recursos dinámicos
- **APIs consistentes**: Misma interfaz para todos los protocolos

### Diferencias Clave
| Aspecto | CoAP | MQTT | HTTP |
|---------|------|------|------|
| **Patrón de suscripción** | Observer con tokens únicos | Event-driven con callbacks | SSE con tareas FreeRTOS |
| **Encoding de payload** | Base64 (mbedTLS) | Texto plano | Texto plano |
| **Transporte** | UDP | TCP | TCP (HTTP/1.1) |
| **Gestión de conexión** | Sesión manual | Event handler automático | Cliente HTTP ESP-IDF |
| **QoS/Confiabilidad** | Confirmable/No-confirmable | QoS 0,1,2 (usando QoS 0) | HTTP status codes |
| **Persistencia** | Observación por token | Conexión única persistente | Conexión SSE por suscripción |

## APIs Disponibles

### CoAP
```c
// Inicializa y conecta al servidor CoAP
void coap_conectar(const char *host, int puerto);

// Suscribe a un tópico con callback asociado (Observer pattern)
void coap_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void coap_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico (cancela observación)
void coap_desuscribir(const char *topico);

// Desconecta y libera recursos
void coap_desconectar();

// Función auxiliar para URI paths multinivel
void coap_agregar_uri_path(coap_pdu_t *pdu, const char *topico);
```

### MQTT
```c
// Inicializa y conecta al broker MQTT
void mqtt_conectar(const char *host, int puerto);

// Suscribe a un tópico con callback asociado
void mqtt_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void mqtt_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico
void mqtt_desuscribir(const char *topico);

// Desconecta y libera el cliente MQTT
void mqtt_desconectar();
```

### HTTP
```c
// Inicializa y conecta al servidor HTTP
void http_conectar(const char *host, int puerto);

// Suscribe a un tópico con callback asociado (SSE)
void http_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void http_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico
void http_desuscribir(const char *topico);

// Desconecta y libera recursos
void http_desconectar();
```

## Formato de Mensajes JSON

Los tres protocolos usan el siguiente formato JSON compatible con el backend Go:

```json
{
  "original": true,
  "topico": "sensor/temperatura",
  "payload": "25.6",
  "interno": false
}
```

### Campos:
- **`original`**: Indica si es mensaje original del dispositivo
- **`topico`**: Tópico de destino/origen
- **`payload`**: Datos del mensaje (base64 en CoAP, texto plano en MQTT/HTTP)
- **`interno`**: Flag para mensajes internos del sistema

## Estructuras de Datos

### CoAP - Observaciones
```c
typedef struct coap_obs {
    char *topico;                    // Tópico observado
    callback_t callback;             // Función callback
    coap_binary_t *token;           // Token único de la observación
    bool activo;                    // Estado de la observación
    STAILQ_ENTRY(coap_obs) entradas; // Lista enlazada
} coap_obs_t;
```

### MQTT - Suscripciones
```c
typedef struct mqtt_sub {
    char *topico;                    // Tópico suscrito
    callback_t callback;             // Función callback
    STAILQ_ENTRY(mqtt_sub) entradas; // Lista enlazada
} mqtt_sub_t;
```

### HTTP - Suscripciones SSE
```c
typedef struct http_sub {
    char *topico;                    // Tópico suscrito
    callback_t callback;             // Función callback
    TaskHandle_t task_handle;        // Handle de tarea SSE
    bool activo;                     // Estado de la suscripción
    STAILQ_ENTRY(http_sub) entradas; // Lista enlazada
} http_sub_t;
```

## Dependencias

### CoAP
- `coap3/coap.h` - Biblioteca libcoap v3
- `mbedtls/base64.h` - Encoding base64
- `cJSON.h` - Manipulación JSON
- `lwip/sockets.h` - Networking UDP

### MQTT
- `mqtt_client.h` - Cliente MQTT ESP-IDF
- `cJSON.h` - Manipulación JSON
- `esp_random.h` - Generación de ClientID único

### HTTP
- `esp_http_client.h` - Cliente HTTP ESP-IDF
- `cJSON.h` - Manipulación JSON
- `freertos/task.h` - Tareas FreeRTOS para SSE

## Estado del Sistema

**✅ SISTEMA COMPLETAMENTE FUNCIONAL**

El sistema está listo para uso en producción con todas las funcionalidades implementadas:
- ✅ Conexión/Desconexión
- ✅ Suscripción/Desuscripción a tópicos
- ✅ Publicación en tópicos
- ✅ Manejo thread-safe
- ✅ Gestión robusta de errores
- ✅ Compatibilidad con backend Go

## Implementación SSE en HTTP

### Características del SSE
- **Headers requeridos**: `Accept: text/event-stream`, `Cache-Control: no-cache`
- **Formato de eventos**: Parsea líneas que comienzan con `data: `
- **Conexiones persistentes**: Una tarea FreeRTOS por suscripción
- **Reconexión automática**: Manejo de desconexiones y errores
- **Integración con callbacks**: Ejecuta callbacks del middleware al recibir datos

### Endpoints HTTP
- **GET** `/sensorwave?topico=X` - Suscripción SSE
- **POST** `/sensorwave` - Publicación de mensajes JSON
- **DELETE** `/sensorwave?topico=X` - Desuscripción

### Flujo SSE
1. Cliente HTTP abre conexión GET con headers SSE
2. Servidor mantiene conexión abierta y envía eventos
3. Cliente parsea eventos `data: ` y ejecuta callbacks
4. Desuscripción cierra conexión y envía DELETE

## Uso Recomendado

1. **Inicialización**: Llamar `*_conectar()` con host y puerto según protocolo
2. **Suscripción**: Usar `*_suscribir()` con callback para recibir mensajes
3. **Publicación**: Usar `*_publicar()` para enviar mensajes
4. **Limpieza**: Llamar `*_desconectar()` al finalizar para liberar recursos

### Selección de Protocolo
- **CoAP**: Ideal para redes con restricciones, UDP, bajo overhead
- **MQTT**: Estándar IoT, TCP confiable, broker centralizado
- **HTTP**: Compatibilidad universal, SSE para tiempo real, fácil debugging

El sistema maneja automáticamente la sincronización, gestión de memoria y formato de mensajes para los tres protocolos.