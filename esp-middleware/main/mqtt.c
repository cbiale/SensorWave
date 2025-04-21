#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "mqtt.h"
#include "sys/queue.h"
#include "esp_log.h"

// Tag del log
#define TAG "MQTT"

// Cliente MQTT
esp_mqtt_client_handle_t cliente;

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

    if (evento->event_id == MQTT_EVENT_DATA) {
        char topico[evento->topic_len + 1];
        char mensaje[evento->data_len + 1];
        memcpy(topico, evento->topic, evento->topic_len);
        memcpy(mensaje, evento->data, evento->data_len);
        topico[evento->topic_len] = '\0';
        mensaje[evento->data_len] = '\0';

        // Verifica si el tópico está registrado
        mqtt_sub_t *actual;
        STAILQ_FOREACH(actual, &lista_subs, entradas) {
            if (strcmp(actual->topico, topico) == 0) {
                // Llama al callback asociado
                actual->callback(topico, mensaje);
                return;
            }
        }

        ESP_LOGI(TAG, "Mensaje recibido en tópico no registrado: %s\n", topico);
    }
}

// Conecta al broker MQTT
void mqtt_conectar(const char *host, int puerto) {
    // Verifica si el cliente ya está conectado
    if (cliente != NULL) {
        ESP_LOGW(TAG, "Cliente MQTT ya conectado. Desconectando...");
        mqtt_desconectar();
    }

    // Armado de la URI
    char uri[512];
    snprintf(uri, sizeof(uri), "mqtt://%s:%d", host, puerto);

    // Datos de configuración
    esp_mqtt_client_config_t mqtt_cfg = {
        .broker.address.uri = uri,
    };

    // Crea un manejador del cliente MQTT
    cliente = esp_mqtt_client_init(&mqtt_cfg);
    if (cliente == NULL) {
        ESP_LOGE(TAG, "Error al inicializar el cliente MQTT\n");
        return;
    }

    // Inicia el cliente MQTT con el manejador creado
    esp_err_t err = esp_mqtt_client_start(cliente);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al conectar al broker MQTT: %s\n", esp_err_to_name(err));
        return;
    }

    // Inicializa la lista de suscripciones
    STAILQ_INIT(&lista_subs);

    // Registra el manejador de eventos del cliente
    esp_mqtt_client_register_event(cliente, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);

    ESP_LOGI(TAG,"Conectado al broker MQTT en %s:%d\n", host, puerto);
}

// Suscribe a un tópico con un callback asociado
void mqtt_suscribir(const char *topico, callback_t cb) {
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente MQTT conectado\n");
        return;
    }

    mqtt_sub_t *actual;
    STAILQ_FOREACH(actual, &lista_subs, entradas) {
        if (strcmp(actual->topico, topico) == 0) {
            actual->callback = cb;
            ESP_LOGI(TAG, "Callback actualizado para el tópico %s", topico);
            return;
        }
    }

    // Solo si no está registrado, se suscribe al broker y guarda en la lista
    esp_err_t err = esp_mqtt_client_subscribe(cliente, topico, 0);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al suscribirse al tópico %s: %s\n", topico, esp_err_to_name(err));
        return;
    }

    mqtt_sub_t *nuevo = malloc(sizeof(mqtt_sub_t));
    nuevo->topico = strdup(topico);
    nuevo->callback = cb;
    STAILQ_INSERT_TAIL(&lista_subs, nuevo, entradas);

    ESP_LOGI(TAG, "Suscrito al tópico %s\n", topico);
}

// Publica un mensaje en un tópico
void mqtt_publicar(const char *topico, const char *mensaje) {
    // Verifica si el cliente está conectado
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente MQTT conectado\n");
        return;
    }

    // Publicar el mensaje en el tópico
    esp_err_t err = esp_mqtt_client_publish(cliente, topico, mensaje, 0, 1, 0);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al publicar en el tópico %s: %s\n", topico, esp_err_to_name(err));
        return;
    }

    ESP_LOGI(TAG, "Mensaje publicado en el tópico %s: %s\n", topico, mensaje);
}

// Desuscribe de un tópico
void mqtt_desuscribir(const char *topico) {
    // Verifica si el cliente está conectado
    if (cliente == NULL) {
        ESP_LOGE(TAG, "No hay cliente MQTT conectado\n");
        return;
    }

    // Desuscribirse del tópico
    esp_err_t err = esp_mqtt_client_unsubscribe(cliente, topico);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Error al desuscribirse del tópico %s: %s\n", topico, esp_err_to_name(err));
        return;
    }
    ESP_LOGI(TAG, "Desuscrito del tópico %s\n", topico);

    mqtt_sub_t *actual, *tmp;
    STAILQ_FOREACH_SAFE(actual, &lista_subs, entradas, tmp) {
        if (strcmp(actual->topico, topico) == 0) {
            STAILQ_REMOVE(&lista_subs, actual, mqtt_sub, entradas);
            free(actual->topico);
            free(actual);
            ESP_LOGI(TAG, "Desuscripción eliminada de la lista");
            return;
        }
    }
}

// Desconecta y libera el cliente MQTT
void mqtt_desconectar() {
    // Verifica si el cliente está conectado
    if (cliente != NULL) {
        esp_mqtt_client_stop(cliente);
        esp_mqtt_client_destroy(cliente);

        // Libera la lista de suscripciones
        mqtt_sub_t *actual, *tmp;
        STAILQ_FOREACH_SAFE(actual, &lista_subs, entradas, tmp) {
            STAILQ_REMOVE(&lista_subs, actual, mqtt_sub, entradas);
            free(actual->topico);
            free(actual);
        }
        cliente = NULL;
        ESP_LOGI(TAG, "Cliente MQTT desconectado y recursos liberados\n");
    } else {
        ESP_LOGE(TAG, "No hay cliente MQTT conectado\n");
    }
}
