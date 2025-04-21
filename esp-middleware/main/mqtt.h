#ifndef MQTT_H
#define MQTT_H

#include "mqtt_client.h"
#include "middleware.h"

// Inicializa y conecta al broker MQTT
void mqtt_conectar(const char *host, int puerto);

// Suscribe a un tópico con un callback asociado
void mqtt_suscribir(const char *topico, callback_t cb);

// Publica un mensaje en un tópico
void mqtt_publicar(const char *topico, const char *mensaje);

// Desuscribe de un tópico
void mqtt_desuscribir(const char *topico);

// Desconecta y libera el cliente MQTT
void mqtt_desconectar();

#endif
