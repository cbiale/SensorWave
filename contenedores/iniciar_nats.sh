#!/bin/bash

# Script para iniciar el servidor NATS.io
echo "Iniciando servidor NATS.io..."

# Verificar si Docker está corriendo
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker no está ejecutándose. Por favor inicia Docker primero."
    exit 1
fi

# Detener y remover contenedor existente si existe
docker stop nats-server 2>/dev/null || true
docker rm nats-server 2>/dev/null || true

# Ejecutar contenedor NATS
docker run -d \
    --name nats-server \
    -p 4222:4222 \
    -p 8222:8222 \
    -p 6222:6222 \
    nats:latest \
    --jetstream \
    --http_port 8222

# Verificar que el contenedor esté corriendo
if [ $? -eq 0 ]; then
    echo "Servidor NATS iniciado exitosamente"
    echo "  - Puerto cliente: 4222"
    echo "  - Puerto monitoreo HTTP: 8222"
    echo "  - Puerto cluster: 6222"
    echo "  - JetStream habilitado"
    echo ""
    echo "Para conectarse: nats://localhost:4222"
    echo "Panel de monitoreo: http://localhost:8222"
else
    echo "Error al iniciar el servidor NATS"
    exit 1
fi