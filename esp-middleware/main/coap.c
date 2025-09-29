#include "coap.h"

#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "sys/queue.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "lwip/arch.h"
#include "lwip/def.h"
#include "lwip/inet.h"
#include "lwip/netdb.h"
#include "lwip/opt.h"
#include "lwip/sockets.h"

#include "cJSON.h"
#include "coap3/coap.h"
#include "mbedtls/base64.h"

// Tag del log
#define TAG "COAP"

// Cliente CoAP
static coap_context_t *contexto = NULL;
static coap_session_t *sesion = NULL;
static coap_address_t direccion_servidor;

// Estado de conexión CoAP
static bool coap_conectado = false;

// Mutex para sincronización
static SemaphoreHandle_t coap_mutex = NULL;

// Estructura para observaciones CoAP
typedef struct coap_obs {
  char *topico;
  callback_t callback;
  coap_binary_t *token;
  bool activo;
  STAILQ_ENTRY(coap_obs) entradas;
} coap_obs_t;

// Lista de observaciones
STAILQ_HEAD(coap_obs_lista_t, coap_obs);
static struct coap_obs_lista_t lista_obs;

// Función para crear mensaje JSON compatible con Go
static char *crear_mensaje_json(const char *topico, const char *payload) {

  size_t largo_payload = strlen(payload);

  // codificar a base64 (ver de controlar reserva de memoria)
  size_t largo_base64;
  mbedtls_base64_encode(NULL, 0, &largo_base64, (const unsigned char *)payload,
                        largo_payload);

  char *base64_payload = malloc(largo_base64);
  mbedtls_base64_encode((unsigned char *)base64_payload, largo_base64,
                        &largo_base64, (const unsigned char *)payload,
                        largo_payload);

  cJSON *json = cJSON_CreateObject();
  cJSON *original = cJSON_CreateBool(true);
  cJSON *topico_json = cJSON_CreateString(topico);
  cJSON *payload_json = cJSON_CreateString(base64_payload); // Base64 encoded
  cJSON *interno = cJSON_CreateBool(false);

  cJSON_AddItemToObject(json, "original", original);
  cJSON_AddItemToObject(json, "topico", topico_json);
  cJSON_AddItemToObject(json, "payload", payload_json);
  cJSON_AddItemToObject(json, "interno", interno);

  char *json_string = cJSON_Print(json);
  cJSON_Delete(json);

  return json_string;
}

// Función auxiliar para agregar URI path con múltiples niveles
void coap_agregar_uri_path(coap_pdu_t *pdu, const char *topico) {
  if (pdu == NULL || topico == NULL) {
    return;
  }

  // Crear copia del tópico para tokenizar
  char *copia = strdup(topico);
  if (copia == NULL) {
    ESP_LOGE(TAG, "Error al asignar memoria para URI path");
    return;
  }

  // Remover '/' inicial si existe
  char *inicio = copia;
  if (inicio[0] == '/') {
    inicio++;
  }

  // Tokenizar por '/'
  char *segmento = strtok(inicio, "/");
  while (segmento != NULL) {
    if (strlen(segmento) > 0) { // Ignorar segmentos vacíos
      coap_add_option(pdu, COAP_OPTION_URI_PATH, strlen(segmento),
                      (const uint8_t *)segmento);
      ESP_LOGD(TAG, "Agregado segmento URI: %s", segmento);
    }
    segmento = strtok(NULL, "/");
  }

  free(copia);
}

// Función para parsear mensaje JSON recibido
static char *parsear_mensaje_json(const char *json_string) {

  cJSON *json = cJSON_Parse(json_string);
  if (json == NULL) {
    ESP_LOGE(TAG, "Error al parsear JSON");
    return NULL;
  }

  // Verificar si es mensaje interno
  cJSON *interno = cJSON_GetObjectItem(json, "interno");
  if (interno != NULL && cJSON_IsTrue(interno)) {
    ESP_LOGI(TAG, "Mensaje interno, ignorando");
    cJSON_Delete(json);
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

// Handler para respuestas CoAP
static coap_response_t coap_response_handler(coap_session_t *session,
                                             const coap_pdu_t *sent,
                                             const coap_pdu_t *received,
                                             const coap_mid_t mid) {
  size_t len;
  const uint8_t *data;

  // Obtener datos de la respuesta
  if (coap_get_data(received, &len, &data)) {
    if (len > 0) {
      char *mensaje = malloc(len + 1);
      if (mensaje != NULL) {
        memcpy(mensaje, data, len);
        mensaje[len] = '\0';

        // Buscar observación correspondiente por token
        coap_bin_const_t token = coap_pdu_get_token(received);

        if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
          coap_obs_t *actual;
          STAILQ_FOREACH(actual, &lista_obs, entradas) {
            ESP_LOGI(TAG, "Procesando respuesta para tópico: %s", actual->topico);
            if (actual->activo && actual->token &&
                actual->token->length == token.length &&
                memcmp(actual->token->s, token.s, token.length) == 0) {

              // Parsear mensaje JSON
              char *payload = parsear_mensaje_json(mensaje);

              if (payload != NULL) {
                xSemaphoreGive(coap_mutex);
                ESP_LOGI(TAG, "Mensaje recibido en el tópico '%s': %s",
                         actual->topico, payload);
                actual->callback(actual->topico, payload);
                free(payload);
                free(mensaje);
                return COAP_RESPONSE_OK;
              } else {
                // Si no es correcto entonces liberar
                xSemaphoreGive(coap_mutex);
                free(mensaje);
                return COAP_RESPONSE_OK;
              }
            }
          }
          xSemaphoreGive(coap_mutex);
        }

        free(mensaje);
      }
    }
  }

  return COAP_RESPONSE_OK;
}

// Agregar esta función para procesar mensajes CoAP
static void coap_task(void *arg) {
  while (coap_conectado) {
    if (contexto != NULL) {
      coap_io_process(contexto, 1000); // Procesar por 1 segundo
    }
    vTaskDelay(10 / portTICK_PERIOD_MS); // Pequeña pausa
  }
  vTaskDelete(NULL);
}

// Inicializa y conecta al servidor CoAP
void coap_conectar(const char *host, int puerto) {
  // Crea el mutex si no existe
  if (coap_mutex == NULL) {
    coap_mutex = xSemaphoreCreateMutex();
    if (coap_mutex == NULL) {
      ESP_LOGE(TAG, "Error al crear mutex CoAP");
      return;
    }
  }
  // Inicializa la lista de observaciones
  STAILQ_INIT(&lista_obs);

  // Inicializa la biblioteca CoAP
  coap_startup();

  memset(&direccion_servidor, 0, sizeof(direccion_servidor));

  struct addrinfo hints, *res;
  memset(&hints, 0, sizeof(hints));
  hints.ai_family = AF_INET;
  hints.ai_socktype = SOCK_DGRAM;

  char puerto_str[10];
  snprintf(puerto_str, sizeof(puerto_str), "%d", puerto);

  int ret = getaddrinfo(host, puerto_str, &hints, &res);
  if (ret != 0) {
    ESP_LOGE(TAG, "Error getaddrinfo");
    return;
  }

  if (res == NULL) {
    ESP_LOGE(TAG, "No se pudo resolver la dirección");
    return;
  }

  // Copiar la dirección resuelta
  memcpy(&direccion_servidor.addr, res->ai_addr, res->ai_addrlen);
  direccion_servidor.size = res->ai_addrlen;
  freeaddrinfo(res);

  // Crear contexto CoAP
  contexto = coap_new_context(NULL);
  if (contexto == NULL) {
    ESP_LOGE(TAG, "Error al crear contexto CoAP");
    return;
  }

  // Crear sesión UDP
  // Si se quiere crear sesión TCP, usar COAP_PROTO_TCP
  sesion = coap_new_client_session(contexto, NULL, &direccion_servidor,
                                   COAP_PROTO_UDP);
  if (sesion == NULL) {
    ESP_LOGE(TAG, "Error al crear sesión CoAP");
    coap_free_context(contexto);
    contexto = NULL;
    return;
  }

  // Registrar handler de respuestas
  coap_register_response_handler(contexto, coap_response_handler);

  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    coap_conectado = true;
    xSemaphoreGive(coap_mutex);
  }

  // Tarea para procesar mensajes CoAP
  xTaskCreate(coap_task, "coap_task", 4096, NULL, 5, NULL);
  ESP_LOGI(TAG, "Cliente CoAP conectado a %s:%d", host, puerto);
}

// Suscribe a un tópico con un callback asociado (Observe)
void coap_suscribir(const char *topico, callback_t cb) {
  if (contexto == NULL || sesion == NULL) {
    ESP_LOGE(TAG, "No hay cliente CoAP inicializado - Suscribir");
    return;
  }

  bool conectado = false;
  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    conectado = coap_conectado;
    xSemaphoreGive(coap_mutex);
  }

  if (!conectado) {
    ESP_LOGE(TAG, "Cliente CoAP no conectado");
    return;
  }

  // Verificar si ya existe la observación
  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    coap_obs_t *actual;
    STAILQ_FOREACH(actual, &lista_obs, entradas) {
      if (strcmp(actual->topico, topico) == 0) {
        actual->callback = cb;
        xSemaphoreGive(coap_mutex);
        ESP_LOGI(TAG, "Callback actualizado para el tópico %s", topico);
        return;
      }
    }
    xSemaphoreGive(coap_mutex);
  }

  // Crear PDU para observación
  coap_pdu_t *pdu = coap_pdu_init(COAP_MESSAGE_CON, COAP_REQUEST_GET,
                                  coap_new_message_id(sesion),
                                  coap_session_max_pdu_size(sesion));
  if (pdu == NULL) {
    ESP_LOGE(TAG, "Error al crear PDU");
    return;
  }

  // Agregar opción Observe
  uint8_t observe_register = 0;
  coap_add_option(pdu, COAP_OPTION_OBSERVE, 1, &observe_register);

  // Agregar URI path con soporte para múltiples niveles
  coap_agregar_uri_path(pdu, topico);

  // Obtener token
  coap_bin_const_t token = coap_pdu_get_token(pdu);

  // Crear nueva observación
  coap_obs_t *nuevo = malloc(sizeof(coap_obs_t));
  if (nuevo == NULL) {
    ESP_LOGE(TAG, "Error al asignar memoria para observación");
    coap_delete_pdu(pdu);
    return;
  }

  nuevo->topico = strdup(topico);
  if (nuevo->topico == NULL) {
    ESP_LOGE(TAG, "Error al asignar memoria para tópico");
    free(nuevo);
    coap_delete_pdu(pdu);
    return;
  }

  nuevo->token = coap_new_binary(token.length);
  if (nuevo->token == NULL) {
    ESP_LOGE(TAG, "Error al asignar memoria para token");
    free(nuevo->topico);
    free(nuevo);
    coap_delete_pdu(pdu);
    return;
  }

  memcpy(nuevo->token->s, token.s, token.length);
  nuevo->callback = cb;
  nuevo->activo = true;

  // Enviar PDU
  coap_mid_t mid = coap_send(sesion, pdu);
  if (mid == COAP_INVALID_MID) {
    ESP_LOGE(TAG, "Error al enviar PDU de observación");
    coap_delete_binary(nuevo->token);
    free(nuevo->topico);
    free(nuevo);
    return;
  }

  // Agregar a la lista
  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    STAILQ_INSERT_TAIL(&lista_obs, nuevo, entradas);
    xSemaphoreGive(coap_mutex);
  }

  ESP_LOGI(TAG, "Observando tópico %s", topico);
}

// Publica un mensaje en un tópico
void coap_publicar(const char *topico, const char *mensaje) {
  if (contexto == NULL || sesion == NULL) {
    ESP_LOGE(TAG, "No hay cliente CoAP inicializado - publicar");
    return;
  }

  bool conectado = false;
  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    conectado = coap_conectado;
    xSemaphoreGive(coap_mutex);
  }

  if (!conectado) {
    ESP_LOGE(TAG, "Cliente CoAP no conectado");
    return;
  }

  // Crear mensaje JSON compatible con Go
  char *json_mensaje = crear_mensaje_json(topico, mensaje);
  if (json_mensaje == NULL) {
    ESP_LOGE(TAG, "Error al crear mensaje JSON");
    return;
  }

  // Crear PDU para POST
  coap_pdu_t *pdu = coap_pdu_init(COAP_MESSAGE_CON, COAP_REQUEST_POST,
                                  coap_new_message_id(sesion),
                                  coap_session_max_pdu_size(sesion));
  if (pdu == NULL) {
    ESP_LOGE(TAG, "Error al crear PDU");
    free(json_mensaje);
    return;
  }

  // Agregar URI path con soporte para múltiples niveles
  coap_agregar_uri_path(pdu, topico);

  // Agregar Content-Format (text/plain)
  uint8_t content_format = COAP_MEDIATYPE_TEXT_PLAIN;
  coap_add_option(pdu, COAP_OPTION_CONTENT_FORMAT, 1, &content_format);

  // Agregar payload
  coap_add_data(pdu, strlen(json_mensaje), (const uint8_t *)json_mensaje);

  // Enviar PDU
  coap_mid_t mid = coap_send(sesion, pdu);
  if (mid == COAP_INVALID_MID) {
    ESP_LOGE(TAG, "Error al enviar PDU de publicación");
    free(json_mensaje);
    return;
  }

  ESP_LOGI(TAG, "Mensaje publicado en el tópico %s", topico);
  free(json_mensaje);
}

// Desuscribe de un tópico (cancela observación)
void coap_desuscribir(const char *topico) {
  if (contexto == NULL || sesion == NULL) {
    ESP_LOGE(TAG, "No hay cliente CoAP inicializado - desuscribir");
    return;
  }

  bool conectado = false;
  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    conectado = coap_conectado;
    xSemaphoreGive(coap_mutex);
  }

  if (!conectado) {
    ESP_LOGE(TAG, "Cliente CoAP no conectado");
    return;
  }

  // Buscar y cancelar observación
  if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
    coap_obs_t *actual, *tmp;
    STAILQ_FOREACH_SAFE(actual, &lista_obs, entradas, tmp) {
      if (strcmp(actual->topico, topico) == 0) {
        actual->activo = false;

        // Crear PDU para cancelar observación
        coap_pdu_t *pdu = coap_pdu_init(COAP_MESSAGE_CON, COAP_REQUEST_GET,
                                        coap_new_message_id(sesion),
                                        coap_session_max_pdu_size(sesion));
        if (pdu != NULL) {
          // Usar el mismo token
          coap_add_token(pdu, actual->token->length, actual->token->s);

          // Agregar opción Observe con valor 1 (deregister)
          uint8_t observe_value = 1;
          coap_add_option(pdu, COAP_OPTION_OBSERVE, 1, &observe_value);

          // Agregar URI path con soporte para múltiples niveles
          coap_agregar_uri_path(pdu, topico);

          // Enviar PDU de cancelación
          coap_mid_t mid = coap_send(sesion, pdu);
          if (mid == COAP_INVALID_MID) {
            ESP_LOGE(TAG, "Error al enviar PDU de cancelación para tópico %s",
                     topico);
          } else {
            ESP_LOGD(TAG, "PDU de cancelación enviado para tópico %s", topico);
          }
        } else {
          ESP_LOGE(TAG, "Error al crear PDU de cancelación para tópico %s",
                   topico);
        }

        STAILQ_REMOVE(&lista_obs, actual, coap_obs, entradas);
        coap_delete_binary(actual->token);
        free(actual->topico);
        free(actual);
        xSemaphoreGive(coap_mutex);

        ESP_LOGI(TAG, "Observación cancelada para tópico %s", topico);
        return;
      }
    }
    xSemaphoreGive(coap_mutex);
  }

  ESP_LOGW(TAG, "Tópico %s no encontrado en observaciones", topico);
}

// Desconecta y libera recursos
void coap_desconectar() {
  if (sesion != NULL || contexto != NULL) {
    // Cancelar todas las observaciones
    if (xSemaphoreTake(coap_mutex, portMAX_DELAY) == pdTRUE) {
      coap_obs_t *actual, *tmp;
      STAILQ_FOREACH_SAFE(actual, &lista_obs, entradas, tmp) {
        actual->activo = false;
        STAILQ_REMOVE(&lista_obs, actual, coap_obs, entradas);
        coap_delete_binary(actual->token);
        free(actual->topico);
        free(actual);
      }
      coap_conectado = false;
      xSemaphoreGive(coap_mutex);
    }

    // Limpiar sesión y contexto
    if (sesion != NULL) {
      coap_session_release(sesion);
      sesion = NULL;
    }

    if (contexto != NULL) {
      coap_free_context(contexto);
      contexto = NULL;
    }

    coap_cleanup();
    ESP_LOGI(TAG, "Cliente CoAP desconectado y recursos liberados");
  } else {
    ESP_LOGE(TAG, "No hay cliente CoAP inicializado - desconectar");
  }
}