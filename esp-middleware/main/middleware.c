#include "middleware.h"
#include "mqtt.h"
#include "coap.h"
#include "http.h"

static Protocolo protocolo_actual;

// Inicializa el middleware con un protocolo especifico
void middleware_conectar(Protocolo protocolo, const char *host, int puerto) {
    protocolo_actual = protocolo;
    switch (protocolo) {
        case MQTT: mqtt_conectar(host, puerto); break;
        case COAP: coap_conectar(host, puerto); break;
        case HTTP: http_conectar(host, puerto); break;
    }
}

// Suscribe a un tópico con un callback asociado
void middleware_suscribir(const char *topico, callback_t cb) {
    switch (protocolo_actual) {
        case MQTT: mqtt_suscribir(topico, cb); break;
        case COAP: coap_suscribir(topico, cb); break;
        case HTTP: http_suscribir(topico, cb); break;
    }
}

// Publica un mensaje en un tópico
void middleware_publicar(const char *topico, const char *mensaje){
    switch (protocolo_actual) {
        case MQTT: mqtt_publicar(topico, mensaje); break;
        case COAP: coap_publicar(topico, mensaje); break;
        case HTTP: http_publicar(topico, mensaje); break;
    }
}

// Desuscribe de un tópico
void middleware_desuscribir(const char *topico) {
    switch (protocolo_actual) {
        case MQTT: mqtt_desuscribir(topico); break;
        case COAP: coap_desuscribir(topico); break;
        case HTTP: http_desuscribir(topico); break;
    }
}

// Desconecta
void middleware_desconectar() {
    switch (protocolo_actual) {
        case MQTT: mqtt_desconectar(); break;
        case COAP: coap_desconectar(); break;
        case HTTP: http_desconectar(); break;
    }
}

