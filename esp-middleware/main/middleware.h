#ifndef MIDDLEWARE_H
#define MIDDLEWARE_H

typedef enum {
    MQTT,
    COAP,
    HTTP
} Protocolo;

// Tipo de callback
typedef void (*callback_t) (const char *topico, const char *mensaje);

// Inicializa el middleware con un protocolo especifico
void middleware_conectar(Protocolo protocolo, const char *host, int puerto);

// Suscribe a un tópico con un callback asociado
void middleware_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void middleware_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico
void middleware_desuscribir(const char *topico);

// Desconecta
void middleware_desconectar();

#endif
