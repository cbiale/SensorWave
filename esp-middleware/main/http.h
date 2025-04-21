#ifndef HTTP_H
#define HTTP_H

#include "middleware.h"

// Inicializa y conecta al servidor HTTP
void http_conectar(const char *host, int puerto);

// Suscribe a un tópico con un callback asociado
void http_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void http_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico
void http_desuscribir(const char *topico);

// Desconecta y libera recursos
void http_desconectar();

#endif
