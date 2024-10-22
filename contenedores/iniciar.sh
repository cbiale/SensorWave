# Contenedor que ejecuta NanoMQ
# Se usa para recibir datos mediante el protocolo MQTT
docker run -d -p 1883:1883 -p 8883:8883 --name nanomq emqx/nanomq:latest

