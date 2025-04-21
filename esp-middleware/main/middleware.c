#include "middleware.h"
#include "mqtt.h"

static Protocolo protocolo_actual;

// Inicializa el middleware con un protocolo especifico
void middleware_conectar(Protocolo protocolo, const char *host, int puerto) {
    protocolo_actual = protocolo;
    switch (protocolo) {
        case MQTT: mqtt_conectar(host, puerto); break;
        case COAP: break;
        case HTTP: break;
    }
}

// Suscribe a un tópico con un callback asociado
void middleware_suscribir(const char *topico, callback_t cb) {
    switch (protocolo_actual) {
        case MQTT: mqtt_suscribir(topico, cb); break;
        case COAP: break;
        case HTTP: break;
    }
}

// Publica un mensaje en un tópico
void middleware_publicar(const char *topico, const char *mensaje){
    switch (protocolo_actual) {
        case MQTT: mqtt_publicar(topico, mensaje); break;
        case COAP: break;
        case HTTP: break;
    }
}

// Desuscribe de un tópico
void middleware_desuscribir(const char *topico) {
    switch (protocolo_actual) {
        case MQTT: break;
        case COAP: break;
        case HTTP: break;
    }
}

// Desconecta
void middleware_desconectar() {
    switch (protocolo_actual) {
        case MQTT: mqtt_desconectar(); break;
        case COAP: break;
        case HTTP: break;
    }
}

