#include <esp_http_client.h>
#include <string.h>

#include "middleware.h"
#include "esp_log.h"
#include "sys/queue.h"

#define TAG "HTTP"                   // Tag del log
#define SSE_REINTENTOS_DELAY_MS 3000 // intentos para reconectarse usando SSE

// estructura de callbacks
typedef struct sse_sub {
    char *topico;
    callback_t cb;
    TaskHandle_t tarea;
    STAILQ_ENTRY(sse_sub) entradas;
} sse_sub_t;

// Define la cabeza de la lista
STAILQ_HEAD(sse_lista_t, sse_sub);
static struct sse_lista_t sse_lista;

char ruta[] = "/sensorwave";  // ruta del cliente HTTP
char servidor_base[512];      // URL del servidor HTTP

// función de manejo de eventos SSE
void sse_task(void *parametros) {
 
    sse_sub_t *sub = (sse_sub_t *)parametros;
    char url[1024];
    snprintf(url, sizeof(url), "%s/%s", servidor_base, sub->topico);

    while (true) {
        // configura el cliente HTTP
        esp_http_client_config_t config = {
            .url = url,
            .method = HTTP_METHOD_GET,
            .disable_auto_redirect = true,
        };

        // inicializa el cliente HTTP
        esp_http_client_handle_t cliente = esp_http_client_init(&config);
        if (!cliente) {
            ESP_LOGE(TAG, "No se pudo inicializar cliente HTTP");
            vTaskDelay(pdMS_TO_TICKS(SSE_REINTENTOS_DELAY_MS));
            continue;
        }

        // configura el manejador de eventos
        esp_err_t err = esp_http_client_open(cliente, 0);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Fallo al conectar SSE: %s", esp_err_to_name(err));
            // libera el cliente HTTP
            esp_http_client_cleanup(cliente);
            vTaskDelay(pdMS_TO_TICKS(SSE_REINTENTOS_DELAY_MS));
            continue;
        }

        ESP_LOGI(TAG, "Conectado a SSE %s", url);

        char buffer[512];
        int len;

        // lee el flujo SSE
        while ((len = esp_http_client_read(cliente, buffer, sizeof(buffer) - 1)) > 0) {
            buffer[len] = '\0';

            if (strncmp(buffer, "data:", 5) == 0) {
                char *payload = buffer + 5;
                while (*payload == ' ') payload++;  // trim leading space
                ESP_LOGI(TAG, "Evento recibido: %s", payload);
                sub->cb(sub->topico, payload);
            }
        }

        // manejo de errores
        ESP_LOGW(TAG, "SSE desconectado, reintentando en %d ms", SSE_REINTENTOS_DELAY_MS);
        esp_http_client_cleanup(cliente);
        vTaskDelay(pdMS_TO_TICKS(SSE_REINTENTOS_DELAY_MS));
    }
}


// Inicializa y conecta al servidor HTTP
void http_conectar(const char *host, int puerto) {
    // armado de la URI (no se necesita configurar el cliente en ESP-IDF)
    snprintf(servidor_base, sizeof(servidor_base), "http://%s:%d", host, puerto);

    // inicializa la lista de suscripciones
    STAILQ_INIT(&sse_lista);

    ESP_LOGI(TAG, "Servidor HTTP configurado en %s", servidor_base);
}

// Suscribe a un tópico con un callback asociado
void http_suscribir(const char *topico, callback_t cb) {
    // verifica si el cliente está conectado
    if (servidor_base[0] == '\0') {
        ESP_LOGE(TAG, "No hay cliente HTTP conectado\n");
        return;
    }

    // verifica si el tópico es válido
    if (strlen(topico) >= 128) {
        ESP_LOGE(TAG, "Tópico demasiado largo\n");
        return;
    }

    sse_sub_t *nueva = malloc(sizeof(sse_sub_t));
    nueva->topico = strdup(topico);
    nueva->cb = cb;

    // lanzar tarea y guardar handle
    xTaskCreate(sse_task, "sse_task", 4096, nueva, 5, &nueva->tarea);

    // insertar al final de la lista
    STAILQ_INSERT_TAIL(&sse_lista, nueva, entradas);
}

// Publica un mensaje en un tópico
void http_publicar(const char *topico, const char *mensaje) {
    // verifica si el cliente está conectado
    if (servidor_base[0] == '\0') {
        ESP_LOGE(TAG, "No hay cliente HTTP conectado\n");
        return;
    }
    // verifica si el mensaje es válido
    if (strlen(mensaje) >= 512) {
        ESP_LOGE(TAG, "Mensaje demasiado largo\n");
        return;
    }
    // verifica si el tópico es válido
    if (strlen(topico) >= 128) {
        ESP_LOGE(TAG, "Tópico demasiado largo\n");
        return;
    }
    // enviar un POST al servidor
    char url[1024];
    snprintf(url, sizeof(url), "%s/%s", servidor_base, topico);
    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_POST,
        .data = mensaje,
        .data_len = strlen(mensaje),
        .disable_auto_redirect = true,
    };
    // inicializa el cliente HTTP
    esp_http_client_handle_t cliente = esp_http_client_init(&config);
    if (!cliente) {
        ESP_LOGE(TAG, "No se pudo inicializar cliente HTTP");
        return;
    }
    // configura el manejador de eventos
    esp_err_t err = esp_http_client_perform(cliente);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al publicar en el tópico %s: %s", topico, esp_err_to_name(err));
    } else {
        ESP_LOGI(TAG, "Mensaje publicado en el tópico %s: %s", topico, mensaje);
    }
    // lee la respuesta
    char buffer[512];
    int len = esp_http_client_read(cliente, buffer, sizeof(buffer) - 1);
    if (len > 0) {
        buffer[len] = '\0';
        ESP_LOGI(TAG, "Respuesta del servidor: %s", buffer);
    } else {
        ESP_LOGE(TAG, "Error al leer la respuesta del servidor");
    }
    // libera el cliente HTTP
    esp_http_client_cleanup(cliente);    
}

// Desuscribe de un tópico
void http_desuscribir(const char *topico) {
    // enviar un DELETE al servidor
    char url[1024];
    snprintf(url, sizeof(url), "%s/%s", servidor_base, topico);

    esp_http_client_config_t config = {
        .url = url,
        .method = HTTP_METHOD_DELETE,
        .disable_auto_redirect = true,
    };

    // inicializa el cliente HTTP
    esp_http_client_handle_t cliente = esp_http_client_init(&config);
    if (!cliente) {
        ESP_LOGE(TAG, "No se pudo inicializar cliente HTTP");
        return;
    }

    // configura el manejador de eventos
    esp_err_t err = esp_http_client_perform(cliente);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al desuscribirse del tópico %s: %s", topico, esp_err_to_name(err));
    } else {
        ESP_LOGI(TAG, "Desuscrito del tópico %s", topico);
    }

    // lee la respuesta
    char buffer[512];
    int len = esp_http_client_read(cliente, buffer, sizeof(buffer) - 1);
    if (len > 0) {
        buffer[len] = '\0';
        ESP_LOGI(TAG, "Respuesta del servidor: %s", buffer);
    } else {
        ESP_LOGE(TAG, "Error al leer la respuesta del servidor");
    }

    // elimina tarea sse asociada
    


    // libera el cliente HTTP
    esp_http_client_cleanup(cliente);
}

// Desconecta y libera recursos
void http_desconectar() {
    // libera el cliente HTTP
    esp_http_client_cleanup(NULL);
    servidor_base[0] = '\0';  // reinicia la URL del servidor
    ESP_LOGI(TAG, "Desconectado del servidor HTTP");
}

