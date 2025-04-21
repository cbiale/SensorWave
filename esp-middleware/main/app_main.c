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
#include "mqtt_client.h"

#include "middleware.h"

// static void mqtt_event_handler(void *handler_args, esp_event_base_t base, int32_t event_id, void *event_data)
// esp_mqtt_client_handle_t client = esp_mqtt_client_init(&mqtt_cfg);
/* The last argument may be used to pass data to the event handler, in this example mqtt_event_handler */
// esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID, mqtt_event_handler, NULL);
//  esp_mqtt_client_start(client);

void mi_funcion(const char *topico, const char *mensaje) {
    printf("Valor recibido");
}

void app_main(void) {

    ESP_ERROR_CHECK(nvs_flash_init());
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    middleware_conectar(MQTT, "localhost", 1883);
    middleware_suscribir("/sensors/temp", mi_funcion);
    middleware_publicar("/sensors/temp", "23");
    middleware_desconectar();
}

