#include "coap_client.h"
#include "coap.h" // Biblioteca CoAP de ESP-IDF
#include "esp_log.h"

static const char *TAG = "COAP_CLIENT";
static coap_context_t *coap_contexto = NULL;
static coap_session_t *coap_sesion = NULL;

// Inicializar el cliente CoAP
esp_err_t coap_conectar (const char *uri_servidor) {
    coap_startup();
    coap_contexto = coap_new_context(NULL);
    if (!coap_contexto) {
        ESP_LOGE(TAG, "Error al crear el contexto CoAP");
        return ESP_FAIL;
    }

    coap_address_t direccion;
    if (coap_split_uri((const uint8_t *)uri_servidor, strlen(server_uri), &direccion) < 0) {
        ESP_LOGE(TAG, "URI inválida: %s", uri_servidor);
        return ESP_FAIL;
    }

    coap_sesion = coap_new_client_session(coap_contexto, NULL, &direccion, COAP_PROTO_UDP);
    if (!coap_sesion) {
        ESP_LOGE(TAG, "Error al crear la sesión CoAP");
        return ESP_FAIL;
    }

    ESP_LOGI(TAG, "Cliente CoAP inicializado con URI: %s", uri_servidor);
    return ESP_OK;
}

// Manejar respuestas del servidor
static void manejar_respuestas(coap_session_t *sesion, const coap_pdu_t *pdu, coap_response_callback_t callback) {
    size_t largo;
    const uint8_t *datos;

    if (coap_get_data(pdu, &largo, &datos)) {
        ESP_LOGI(TAG, "Respuesta recibida: %.*s", (int)largo, datos);
        if (callback) {
            callback((const char *)datos, largo);
        }
    }
}

// Publicar
esp_err_t coap_publicar(const char *topico, const char *payload, coap_response_callback_t callback) {
    coap_pdu_t *pdu = coap_new_pdu(coap_sesion);
    if (!pdu) {
        ESP_LOGE(TAG, "Error al crear el PDU");
        return ESP_FAIL;
    }

    coap_add_option(pdu, COAP_OPTION_URI_PATH, strlen(topico), (const uint8_t *)topico);
    coap_add_data(pdu, strlen(payload), (const uint8_t *)payload);
    coap_send(coap_session, pdu);

    coap_run_once(coap_contexto, 0); // Procesar la respuesta
    handle_response(coap_sesion, coap_sesion->last_pdu, callback);

    return ESP_OK;
}

// Suscribir a un recurso
esp_err_t coap_suscribir(const char *topico, coap_response_callback_t callback) {
    coap_pdu_t *pdu = coap_new_pdu(coap_sesion);
    if (!pdu) {
        ESP_LOGE(TAG, "Error al crear el PDU");
        return ESP_FAIL;
    }

    coap_add_option(pdu, COAP_OPTION_URI_PATH, strlen(topico), (const uint8_t *)topico);
    coap_add_option(pdu, COAP_OPTION_OBSERVE, 0, NULL);
    coap_send(coap_sesion, pdu);

    while (1) {
        coap_run_once(coap_contexto, 0); // Procesar notificaciones
        handle_response(coap_sesion, coap_sesion->last_pdu, callback);
    }

    return ESP_OK;
}

// Cancelar una observacion
esp_err_t coap_cancelar_observacion(const char *topico) {
    coap_pdu_t *pdu = coap_new_pdu(coap_sesion);
    if (!pdu) {
        ESP_LOGE(TAG, "Error al crear el PDU");
        return ESP_FAIL;
    }

    coap_add_option(pdu, COAP_OPTION_URI_PATH, strlen(topico), (const uint8_t *)topico);
    coap_add_option(pdu, COAP_OPTION_OBSERVE, 0, NULL);
    coap_send(coap_sesion, pdu);

    return ESP_OK;
}

// Desconectar el cliente CoAP
void coap_desconectar() {
    if (coap_sesion) {
        coap_session_release(coap_sesion);
        coap_sesion = NULL;
    }
    if (coap_contexto) {
        coap_free_context(coap_contexto);
        coap_contexto = NULL;
    }
    coap_cleanup();
    ESP_LOGI(TAG, "Cliente CoAP desconectado");
}