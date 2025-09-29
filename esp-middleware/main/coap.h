#ifndef COAP_H
#define COAP_H

#include "middleware.h"

// Inicializa y conecta al servidor CoAP
void coap_conectar(const char *host, int puerto);

// Suscribe a un tópico con un callback asociado
void coap_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void coap_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico
void coap_desuscribir(const char *topico);

// Desconecta y libera recursos
void coap_desconectar();


#endif
