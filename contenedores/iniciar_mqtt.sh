#!/bin/bash

# Script para iniciar el servidor MQTT NanoMQ
echo "Iniciando servidor MQTT NanoMQ..."

# Verificar si Docker está corriendo
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker no está ejecutándose. Por favor inicia Docker primero."
    exit 1
fi

# Detener y remover contenedor existente si existe
docker stop nanomq 2>/dev/null || true
docker rm nanomq 2>/dev/null || true

# Ejecutar contenedor NanoMQ
docker run -d \
    --name nanomq \
    -p 1883:1883 \
    -p 8883:8883 \
    emqx/nanomq:latest

# Verificar que el contenedor esté corriendo
if [ $? -eq 0 ]; then
    echo "Servidor MQTT NanoMQ iniciado exitosamente"
    echo "  - Puerto MQTT: 1883"
    echo "  - Puerto MQTT SSL: 8883"
    echo ""
    echo "Para conectarse: mqtt://localhost:1883"
else
    echo "Error al iniciar el servidor MQTT"
    exit 1
fi
