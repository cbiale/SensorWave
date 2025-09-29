#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "mqtt.h"
#include "sys/queue.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "cJSON.h"
#include "esp_random.h"
#include <inttypes.h>

// Tag del log
#define TAG "MQTT"

// Cliente MQTT
esp_mqtt_client_handle_t cliente;

// Estado de conexión MQTT
static bool mqtt_conectado = false;

// Mutex para sincronización
static SemaphoreHandle_t mqtt_mutex = NULL;

// Función para generar ClientID único
static char* generar_client_id() {
    static char client_id[64];
    uint32_t random_num = esp_random();
    snprintf(client_id, sizeof(client_id), "sensorwave_%08" PRIx32, random_num);
    return client_id;
}

// Función para crear mensaje JSON compatible con Go
static char* crear_mensaje_json(const char *topico, const char *payload) {
    cJSON *json = cJSON_CreateObject();
    cJSON *original = cJSON_CreateBool(true);
    cJSON *topico_json = cJSON_CreateString(topico);
    cJSON *payload_json = cJSON_CreateString(payload);
    cJSON *interno = cJSON_CreateBool(false);
    
    cJSON_AddItemToObject(json, "original", original);
    cJSON_AddItemToObject(json, "topico", topico_json);
    cJSON_AddItemToObject(json, "payload", payload_json);
    cJSON_AddItemToObject(json, "interno", interno);
    
    char *json_string = cJSON_Print(json);
    cJSON_Delete(json);
    
    return json_string;
}

// Función para parsear mensaje JSON recibido
static char* parsear_mensaje_json(const char *json_string) {
    cJSON *json = cJSON_Parse(json_string);
    if (json == NULL) {
        ESP_LOGE(TAG, "Error al parsear JSON");
        return NULL;
    }
    
    cJSON *payload = cJSON_GetObjectItem(json, "payload");
    if (payload == NULL || !cJSON_IsString(payload)) {
        ESP_LOGE(TAG, "Campo 'payload' no encontrado o no es string");
        cJSON_Delete(json);
        return NULL;
    }
    
    char *payload_str = strdup(cJSON_GetStringValue(payload));
    cJSON_Delete(json);
    
    return payload_str;
}

// Tipo de datos que almacena el callback por tópico 
typedef struct mqtt_sub {
    char *topico;
    callback_t callback;
    STAILQ_ENTRY(mqtt_sub) entradas;
} mqtt_sub_t;

// Cabeza de la lista
STAILQ_HEAD(mqtt_sub_lista_t, mqtt_sub);
static struct mqtt_sub_lista_t lista_subs;

// Dispatcher global para eventos MQTT
static void mqtt_event_handler(void *handler_args, esp_event_base_t base, int32_t event_id, void *event_data) {
    esp_mqtt_event_handle_t evento = event_data;

    switch (evento->event_id) {
        case MQTT_EVENT_CONNECTED:
            ESP_LOGI(TAG, "Cliente MQTT conectado al broker");
            if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
                mqtt_conectado = true;
                ESP_LOGI(TAG, "Variblae mqtt_conectado actualizada a true");
                xSemaphoreGive(mqtt_mutex);
            }
            break;

        case MQTT_EVENT_DISCONNECTED:
            ESP_LOGW(TAG, "Cliente MQTT desconectado del broker");
            if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
                mqtt_conectado = false;
                xSemaphoreGive(mqtt_mutex);
            }
            break;

        case MQTT_EVENT_SUBSCRIBED:
            ESP_LOGI(TAG, "Suscripción exitosa, msg_id=%d", evento->msg_id);
            break;

        case MQTT_EVENT_UNSUBSCRIBED:
            ESP_LOGI(TAG, "Desuscripción exitosa, msg_id=%d", evento->msg_id);
            break;

        case MQTT_EVENT_PUBLISHED:
            ESP_LOGI(TAG, "Mensaje publicado exitosamente, msg_id=%d", evento->msg_id);
            break;

        case MQTT_EVENT_DATA:
            {
                char topico[evento->topic_len + 1];
                char mensaje[evento->data_len + 1];
                memcpy(topico, evento->topic, evento->topic_len);
                memcpy(mensaje, evento->data, evento->data_len);
                topico[evento->topic_len] = '\0';
                mensaje[evento->data_len] = '\0';

                // Verifica si el tópico está registrado
                if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
                    mqtt_sub_t *actual;
                    callback_t callback_encontrado = NULL;
                    
                    STAILQ_FOREACH(actual, &lista_subs, entradas) {
                        if (strcmp(actual->topico, topico) == 0) {
                            callback_encontrado = actual->callback;
                            break;
                        }
                    }
                    xSemaphoreGive(mqtt_mutex);
                    
                    if (callback_encontrado != NULL) {
                        // Parsear mensaje JSON para extraer payload
                        char *payload = parsear_mensaje_json(mensaje);
                        if (payload != NULL) {
                            // Llama al callback con el payload extraído
                            callback_encontrado(topico, payload);
                            free(payload);
                        } else {
                            // Si no es JSON válido, usar mensaje original
                            callback_encontrado(topico, mensaje);
                        }
                    } else {
                        ESP_LOGI(TAG, "Mensaje recibido en tópico no manejado: %s", topico);
                    }
                }
            }
            break;

        case MQTT_EVENT_ERROR:
            ESP_LOGE(TAG, "Error en cliente MQTT");
            if (evento->error_handle->error_type == MQTT_ERROR_TYPE_TCP_TRANSPORT) {
                ESP_LOGE(TAG, "Error de transporte TCP reportado desde esp-tls: 0x%x", evento->error_handle->esp_tls_last_esp_err);
                ESP_LOGE(TAG, "Error de transporte TCP reportado desde tls stack: 0x%x", evento->error_handle->esp_tls_stack_err);
                ESP_LOGE(TAG, "Error de transporte TCP reportado desde socket errno: %d", evento->error_handle->esp_transport_sock_errno);
            } else if (evento->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED) {
                ESP_LOGE(TAG, "Conexión rechazada por el broker, código de error: 0x%x", evento->error_handle->connect_return_code);
            }
            break;

        default:
            ESP_LOGW(TAG, "Evento MQTT no manejado: %d", evento->event_id);
            break;
    }
}

// Conecta al broker MQTT
void mqtt_conectar(const char *host, int puerto) {
    // Crea el mutex si no existe
    if (mqtt_mutex == NULL) {
        mqtt_mutex = xSemaphoreCreateMutex();
        if (mqtt_mutex == NULL) {
            ESP_LOGE(TAG, "Error al crear mutex MQTT");
            return;
        }
    }

    // Inicializa la lista de suscripciones
    STAILQ_INIT(&lista_subs);

    // Verifica si el cliente ya está conectado
    if (cliente != NULL) {
        ESP_LOGW(TAG, "Cliente MQTT ya conectado. Desconectando...");
        mqtt_desconectar();
    }

    // Armado de la URI
    char uri[512];
    snprintf(uri, sizeof(uri), "mqtt://%s:%d", host, puerto);

    // Generar ClientID único
    char *client_id = generar_client_id();
    
    // Datos de configuración
    esp_mqtt_client_config_t mqtt_cfg = {
        .broker.address.uri = uri,
        .credentials.client_id = client_id,
    };

    // Crea un manejador del cliente MQTT
    cliente = esp_mqtt_client_init(&mqtt_cfg);
    if (cliente == NULL) {
        ESP_LOGE(TAG, "Error al inicializar el cliente MQTT\n");
        return;
    }

    // Registra el manejador de eventos del cliente
    esp_mqtt_client_register_event(cliente, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);

    // Inicia el cliente MQTT con el manejador creado
    esp_err_t err = esp_mqtt_client_start(cliente);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al conectar al broker MQTT: %s\n", esp_err_to_name(err));
        return;
    }

    ESP_LOGI(TAG,"Conectado al broker MQTT en %s:%d\n", host, puerto);
}

// Suscribe a un tópico con un callback asociado
void mqtt_suscribir(const char *topico, callback_t cb) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente MQTT inicializado");
        return;
    }

    bool conectado = false;
    if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
        conectado = mqtt_conectado;
        xSemaphoreGive(mqtt_mutex);
    }

    if (!conectado) {
        ESP_LOGE(TAG, "Cliente MQTT no conectado al broker");
        return;
    }

    if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
        mqtt_sub_t *actual;
        // Verifica si el tópico ya está registrado
        // Si está registrado, actualiza el callback
        STAILQ_FOREACH(actual, &lista_subs, entradas) {
            if (strcmp(actual->topico, topico) == 0) {
                actual->callback = cb;
                xSemaphoreGive(mqtt_mutex);
                ESP_LOGI(TAG, "Callback actualizado para el tópico %s", topico);
                return;
            }
        }
        xSemaphoreGive(mqtt_mutex);
    }

    ESP_LOGI(TAG, "Suscribiendo al tópico %s", topico);
    
    // Solo si no está registrado, se suscribe al broker y guarda en la lista
    esp_err_t err = esp_mqtt_client_subscribe(cliente, topico, 0);
    if (err < 0) {
        ESP_LOGE(TAG, "Error al suscribirse al tópico %s: %s\n", topico, esp_err_to_name(err));
        return;
    }

    mqtt_sub_t *nuevo = malloc(sizeof(mqtt_sub_t));
    if (nuevo == NULL) {
        ESP_LOGE(TAG, "Error al asignar memoria para suscripción");
        return;
    }

    nuevo->topico = strdup(topico);
    if (nuevo->topico == NULL) {
        ESP_LOGE(TAG, "Error al asignar memoria para tópico");
        free(nuevo);
        return;
    }

    nuevo->callback = cb;
    
    if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
        STAILQ_INSERT_TAIL(&lista_subs, nuevo, entradas);
        xSemaphoreGive(mqtt_mutex);
    }

    ESP_LOGI(TAG, "Suscrito al tópico %s\n", topico);
}

// Publica un mensaje en un tópico
void mqtt_publicar(const char *topico, const char *mensaje) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente MQTT inicializado");
        return;
    }

    bool conectado = false;
    if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
        conectado = mqtt_conectado;
        xSemaphoreGive(mqtt_mutex);
    }

    if (!conectado) {
        ESP_LOGE(TAG, "Cliente MQTT no conectado al broker");
        return;
    }

    // Crear mensaje JSON compatible con Go
    char *json_mensaje = crear_mensaje_json(topico, mensaje);
    if (json_mensaje == NULL) {
        ESP_LOGE(TAG, "Error al crear mensaje JSON");
        return;
    }

    // Publicar el mensaje JSON en el tópico
    esp_err_t err = esp_mqtt_client_publish(cliente, topico, json_mensaje, strlen(json_mensaje), 0, false);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al publicar en el tópico %s: %s", topico, esp_err_to_name(err));
        free(json_mensaje);
        return;
    }

    ESP_LOGI(TAG, "Mensaje publicado en el tópico %s: %s", topico, mensaje);
    free(json_mensaje);
}

// Desuscribe de un tópico
void mqtt_desuscribir(const char *topico) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente MQTT inicializado");
        return;
    }

    bool conectado = false;
    if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
        conectado = mqtt_conectado;
        xSemaphoreGive(mqtt_mutex);
    }

    if (!conectado) {
        ESP_LOGE(TAG, "Cliente MQTT no conectado al broker");
        return;
    }

    // Desuscribirse del tópico
    esp_err_t err = esp_mqtt_client_unsubscribe(cliente, topico);
    if (err < 0) {
        ESP_LOGE(TAG, "Error al desuscribirse del tópico %s: %s\n", topico, esp_err_to_name(err));
        return;
    }
    ESP_LOGI(TAG, "Desuscrito del tópico %s\n", topico);

    if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
        mqtt_sub_t *actual, *tmp;
        STAILQ_FOREACH_SAFE(actual, &lista_subs, entradas, tmp) {
            if (strcmp(actual->topico, topico) == 0) {
                STAILQ_REMOVE(&lista_subs, actual, mqtt_sub, entradas);
                free(actual->topico);
                free(actual);
                xSemaphoreGive(mqtt_mutex);
                ESP_LOGI(TAG, "Desuscripción eliminada de la lista");
                return;
            }
        }
        xSemaphoreGive(mqtt_mutex);
    }
}

// Desconecta y libera el cliente MQTT
void mqtt_desconectar() {
    if (cliente != NULL) {
        esp_mqtt_client_stop(cliente);
        esp_mqtt_client_destroy(cliente);

        // Libera la lista de suscripciones
        if (xSemaphoreTake(mqtt_mutex, portMAX_DELAY) == pdTRUE) {
            mqtt_sub_t *actual, *tmp;
            STAILQ_FOREACH_SAFE(actual, &lista_subs, entradas, tmp) {
                STAILQ_REMOVE(&lista_subs, actual, mqtt_sub, entradas);
                free(actual->topico);
                free(actual);
            }
            mqtt_conectado = false;
            xSemaphoreGive(mqtt_mutex);
        }
        cliente = NULL;
        ESP_LOGI(TAG, "Cliente MQTT desconectado y recursos liberados");
    } else {
        ESP_LOGE(TAG, "No hay cliente MQTT inicializado");
    }
}
