#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "http.h"
#include "sys/queue.h"
#include "esp_log.h"
#include "esp_http_client.h"
#include "cJSON.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"

// Tag del log
#define TAG "HTTP"

// Ruta del endpoint
#define HTTP_RUTA "/sensorwave"

// Cliente HTTP
static esp_http_client_handle_t cliente = NULL;
static char base_url[256] = {0};

// Estado de conexión HTTP
static bool http_conectado = false;

// Mutex para sincronización
static SemaphoreHandle_t http_mutex = NULL;

// Estructura para suscripciones HTTP
typedef struct http_sub {
    char *topico;
    callback_t callback;
    TaskHandle_t task_handle;
    bool activo;
    STAILQ_ENTRY(http_sub) entradas;
} http_sub_t;

// Lista de suscripciones
STAILQ_HEAD(http_sub_lista_t, http_sub);
static struct http_sub_lista_t lista_subs;

// Función para crear mensaje JSON
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

// Callback para respuestas HTTP
static esp_err_t http_event_handler(esp_http_client_event_t *evt) {
    switch (evt->event_id) {
        case HTTP_EVENT_ERROR:
            ESP_LOGE(TAG, "Error HTTP");
            break;
        case HTTP_EVENT_ON_CONNECTED:
            ESP_LOGI(TAG, "Cliente HTTP conectado");
            break;
        case HTTP_EVENT_HEADER_SENT:
            ESP_LOGD(TAG, "Headers enviados");
            break;
        case HTTP_EVENT_ON_HEADER:
            ESP_LOGD(TAG, "Header recibido: %s: %s", evt->header_key, evt->header_value);
            break;
        case HTTP_EVENT_ON_DATA:
            ESP_LOGD(TAG, "Datos recibidos: %.*s", evt->data_len, (char*)evt->data);
            break;
        case HTTP_EVENT_ON_FINISH:
            ESP_LOGD(TAG, "Petición HTTP finalizada");
            break;
        case HTTP_EVENT_DISCONNECTED:
            ESP_LOGI(TAG, "Cliente HTTP desconectado");
            break;
        case HTTP_EVENT_REDIRECT:
            ESP_LOGI(TAG, "Redirección HTTP");
            break;
    }
    return ESP_OK;
}

// Tarea para manejar suscripciones SSE
static void http_sse_task(void *pvParameters) {
    http_sub_t *sub = (http_sub_t*)pvParameters;
    
    char url[512];
    snprintf(url, sizeof(url), "%s%s?topico=%s", base_url, HTTP_RUTA, sub->topico);
    
    esp_http_client_config_t config = {
        .url = url,
        .event_handler = http_event_handler,
        .timeout_ms = 5000,
    };
    
    esp_http_client_handle_t client = esp_http_client_init(&config);
    
    if (client == NULL) {
        ESP_LOGE(TAG, "Error al inicializar cliente HTTP para SSE");
        vTaskDelete(NULL);
        return;
    }
    
    esp_http_client_set_method(client, HTTP_METHOD_GET);
    esp_http_client_set_header(client, "Accept", "text/event-stream");
    esp_http_client_set_header(client, "Cache-Control", "no-cache");
    
    ESP_LOGI(TAG, "Iniciando conexión SSE para tópico: %s", sub->topico);
    
    esp_err_t err = esp_http_client_open(client, 0);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al abrir conexión SSE: %s", esp_err_to_name(err));
        esp_http_client_cleanup(client);
        vTaskDelete(NULL);
        return;
    }
    
    char buffer[1024];
    while (sub->activo) {
        int data_read = esp_http_client_read(client, buffer, sizeof(buffer) - 1);
        if (data_read > 0) {
            buffer[data_read] = '\0';
            
            // Procesar SSE
            char *line = strtok(buffer, "\n");
            while (line != NULL && sub->activo) {
                if (strncmp(line, "data: ", 6) == 0) {
                    char *data = line + 6;
                    // Eliminar \r si existe
                    char *cr = strchr(data, '\r');
                    if (cr) *cr = '\0';
                    
                    if (sub->callback) {
                        sub->callback(sub->topico, data);
                    }
                }
                line = strtok(NULL, "\n");
            }
        } else if (data_read == 0) {
            // Conexión cerrada
            ESP_LOGI(TAG, "Conexión SSE cerrada para tópico: %s", sub->topico);
            break;
        } else {
            ESP_LOGE(TAG, "Error al leer datos SSE: %d", data_read);
            break;
        }
        
        vTaskDelay(pdMS_TO_TICKS(10));
    }
    
    esp_http_client_close(client);
    esp_http_client_cleanup(client);
    ESP_LOGI(TAG, "Tarea SSE terminada para tópico: %s", sub->topico);
    vTaskDelete(NULL);
}

// Inicializa y conecta al servidor HTTP
void http_conectar(const char *host, int puerto) {
    // Crea el mutex si no existe
    if (http_mutex == NULL) {
        http_mutex = xSemaphoreCreateMutex();
        if (http_mutex == NULL) {
            ESP_LOGE(TAG, "Error al crear mutex HTTP");
            return;
        }
    }
    
    // Inicializa la lista de suscripciones
    STAILQ_INIT(&lista_subs);
    
    // Construye la URL base
    snprintf(base_url, sizeof(base_url), "http://%s:%d", host, puerto);
    
    // Configuración del cliente HTTP
    esp_http_client_config_t config = {
        .url = base_url,
        .event_handler = http_event_handler,
        .timeout_ms = 5000,
    };
    
    cliente = esp_http_client_init(&config);
    if (cliente == NULL) {
        ESP_LOGE(TAG, "Error al inicializar cliente HTTP");
        return;
    }
    
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        http_conectado = true;
        xSemaphoreGive(http_mutex);
    }
    
    ESP_LOGI(TAG, "Cliente HTTP conectado a %s:%d", host, puerto);
}

// Suscribe a un tópico con un callback asociado
void http_suscribir(const char *topico, callback_t cb) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente HTTP inicializado");
        return;
    }
    
    bool conectado = false;
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        conectado = http_conectado;
        xSemaphoreGive(http_mutex);
    }
    
    if (!conectado) {
        ESP_LOGE(TAG, "Cliente HTTP no conectado");
        return;
    }
    
    // Verificar si ya existe la suscripción
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        http_sub_t *actual;
        STAILQ_FOREACH(actual, &lista_subs, entradas) {
            if (strcmp(actual->topico, topico) == 0) {
                actual->callback = cb;
                xSemaphoreGive(http_mutex);
                ESP_LOGI(TAG, "Callback actualizado para el tópico %s", topico);
                return;
            }
        }
        xSemaphoreGive(http_mutex);
    }
    
    // Crear nueva suscripción
    http_sub_t *nuevo = malloc(sizeof(http_sub_t));
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
    nuevo->activo = true;
    
    // Crear tarea SSE
    BaseType_t result = xTaskCreate(http_sse_task, "http_sse", 4096, nuevo, 5, &nuevo->task_handle);
    if (result != pdPASS) {
        ESP_LOGE(TAG, "Error al crear tarea SSE");
        free(nuevo->topico);
        free(nuevo);
        return;
    }
    
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        STAILQ_INSERT_TAIL(&lista_subs, nuevo, entradas);
        xSemaphoreGive(http_mutex);
    }
    
    ESP_LOGI(TAG, "Suscrito al tópico %s", topico);
}

// Publica un mensaje en un tópico
void http_publicar(const char *topico, const char *mensaje) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente HTTP inicializado");
        return;
    }
    
    bool conectado = false;
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        conectado = http_conectado;
        xSemaphoreGive(http_mutex);
    }
    
    if (!conectado) {
        ESP_LOGE(TAG, "Cliente HTTP no conectado");
        return;
    }
    
    // Crear JSON del mensaje
    char *json_data = crear_mensaje_json(topico, mensaje);
    if (json_data == NULL) {
        ESP_LOGE(TAG, "Error al crear JSON del mensaje");
        return;
    }
    
    // Configurar URL completa
    char url[512];
    snprintf(url, sizeof(url), "%s%s", base_url, HTTP_RUTA);
    
    esp_http_client_set_url(cliente, url);
    esp_http_client_set_method(cliente, HTTP_METHOD_POST);
    esp_http_client_set_header(cliente, "Content-Type", "application/json");
    esp_http_client_set_post_field(cliente, json_data, strlen(json_data));
    
    esp_err_t err = esp_http_client_perform(cliente);
    if (err == ESP_OK) {
        int status_code = esp_http_client_get_status_code(cliente);
        if (status_code == 200) {
            ESP_LOGI(TAG, "Mensaje publicado en el tópico %s", topico);
        } else {
            ESP_LOGE(TAG, "Error HTTP: %d", status_code);
        }
    } else {
        ESP_LOGE(TAG, "Error al realizar POST: %s", esp_err_to_name(err));
    }
    
    free(json_data);
}

// Desuscribe de un tópico
void http_desuscribir(const char *topico) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente HTTP inicializado");
        return;
    }
    
    bool conectado = false;
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        conectado = http_conectado;
        xSemaphoreGive(http_mutex);
    }
    
    if (!conectado) {
        ESP_LOGE(TAG, "Cliente HTTP no conectado");
        return;
    }
    
    // Buscar y detener la suscripción
    if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
        http_sub_t *actual, *tmp;
        STAILQ_FOREACH_SAFE(actual, &lista_subs, entradas, tmp) {
            if (strcmp(actual->topico, topico) == 0) {
                actual->activo = false;
                
                // Detener la tarea SSE
                if (actual->task_handle != NULL) {
                    vTaskDelete(actual->task_handle);
                }
                
                STAILQ_REMOVE(&lista_subs, actual, http_sub, entradas);
                free(actual->topico);
                free(actual);
                xSemaphoreGive(http_mutex);
                
                // Enviar DELETE al servidor
                char url[512];
                snprintf(url, sizeof(url), "%s%s?topico=%s", base_url, HTTP_RUTA, topico);
                
                esp_http_client_set_url(cliente, url);
                esp_http_client_set_method(cliente, HTTP_METHOD_DELETE);
                
                esp_err_t err = esp_http_client_perform(cliente);
                if (err == ESP_OK) {
                    int status_code = esp_http_client_get_status_code(cliente);
                    if (status_code == 200) {
                        ESP_LOGI(TAG, "Desuscrito del tópico %s", topico);
                    } else {
                        ESP_LOGE(TAG, "Error HTTP al desuscribir: %d", status_code);
                    }
                } else {
                    ESP_LOGE(TAG, "Error al realizar DELETE: %s", esp_err_to_name(err));
                }
                
                return;
            }
        }
        xSemaphoreGive(http_mutex);
    }
    
    ESP_LOGW(TAG, "Tópico %s no encontrado en suscripciones", topico);
}

// Desconecta y libera recursos
void http_desconectar() {
    if (cliente != NULL) {
        // Detener todas las suscripciones
        if (xSemaphoreTake(http_mutex, portMAX_DELAY) == pdTRUE) {
            http_sub_t *actual, *tmp;
            STAILQ_FOREACH_SAFE(actual, &lista_subs, entradas, tmp) {
                actual->activo = false;
                
                if (actual->task_handle != NULL) {
                    vTaskDelete(actual->task_handle);
                }
                
                STAILQ_REMOVE(&lista_subs, actual, http_sub, entradas);
                free(actual->topico);
                free(actual);
            }
            http_conectado = false;
            xSemaphoreGive(http_mutex);
        }
        
        esp_http_client_cleanup(cliente);
        cliente = NULL;
        ESP_LOGI(TAG, "Cliente HTTP desconectado y recursos liberados");
    } else {
        ESP_LOGE(TAG, "No hay cliente HTTP inicializado");
    }
}