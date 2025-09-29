#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <stdlib.h>
#include <inttypes.h>
#include "esp_system.h"
#include "nvs_flash.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "esp_log.h"

#include "esp_wifi.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include "middleware.h"

// Configuración WiFi
#define WIFI_SSID "TP-Link_9471"
#define WIFI_PASSWORD "07737145"

static void wifi_event_handler(void* arg, esp_event_base_t event_base,
                              int32_t event_id, void* event_data) {
    if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
    } else if (event_base == WIFI_EVENT && event_id == WIFI_EVENT_STA_DISCONNECTED) {
        esp_wifi_connect();
    } else if (event_base == IP_EVENT && event_id == IP_EVENT_STA_GOT_IP) {
        ip_event_got_ip_t* event = (ip_event_got_ip_t*) event_data;
        ESP_LOGI("WIFI", "Got IP:" IPSTR, IP2STR(&event->ip_info.ip));
    }
}

void wifi_init(void) {
    // Crear la interfaz WiFi por defecto
    esp_netif_create_default_wifi_sta();

    // Configurar WiFi
    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    // Iniciar WiFi con la configuración por defecto
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    // Registrar manejadores de eventos para WiFi y IP
    esp_event_handler_instance_t instance_any_id;
    esp_event_handler_instance_t instance_got_ip;
    ESP_ERROR_CHECK(esp_event_handler_instance_register(WIFI_EVENT,
        ESP_EVENT_ANY_ID, &wifi_event_handler, NULL, &instance_any_id));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(IP_EVENT,
        IP_EVENT_STA_GOT_IP, &wifi_event_handler, NULL, &instance_got_ip));

    // Configurar la conexión WiFi
    wifi_config_t wifi_config = {
        .sta = {
            .ssid = WIFI_SSID,
            .password = WIFI_PASSWORD,
        },
    };
    // Configurar el modo WiFi como estación (STA)
    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));

    // Establecer la configuración WiFi
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifi_config));
    // Iniciar WiFi
    ESP_ERROR_CHECK(esp_wifi_start());
}

void mi_funcion(const char *topico, const char *mensaje) {
    printf("Valor recibido en el tópico '%s': %s\n", topico, mensaje);
}

// sacar recepción del mensaje con interno = true
// verificar que se manejen bytes en mensaje y no string (ok)
// En static char* parsear_mensaje_json(const char *json_string) verificar si devuelvo char o bytes

void app_main(void) {

    // Inicializar NVS (Non-Volatile Storage)
    ESP_ERROR_CHECK(nvs_flash_init());
    // Inicializa el stack TCP/IP
    ESP_ERROR_CHECK(esp_netif_init());
    // Crear el bucle de eventos por defecto
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    // Inicializar la red WiFi
    wifi_init();

    // conectar al broker MQTT
    middleware_conectar(COAP, "192.168.0.105", 5683);
    // middleware_conectar(MQTT, "192.168.0.105", 1883);
    // Esperar un poco más para la conexión
    vTaskDelay(20000 / portTICK_PERIOD_MS);

    // suscribir al topic sensores/temperatura
    // y registrar la función mi_funcion como callback
    middleware_suscribir("/sensores/temperatura", mi_funcion);
    // dormir 10 segundos
    vTaskDelay(10000 / portTICK_PERIOD_MS);
    // publicar un mensaje en el topic sensores/temperatura
    middleware_publicar("/sensores/temperatura", "23");
    // dormir 10 segundos
    vTaskDelay(10000 / portTICK_PERIOD_MS);
    middleware_desuscribir("/sensores/temperatura");
    // dormir 10 segundos
    vTaskDelay(10000 / portTICK_PERIOD_MS);
    middleware_publicar("/sensores/temperatura", "25");
    // dormir 10 segundos
    vTaskDelay(10000 / portTICK_PERIOD_MS);
    // desconectar del broker
    middleware_desconectar();
}

