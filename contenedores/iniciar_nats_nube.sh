#!/bin/bash

# Script para iniciar servidor NATS.io simulando servicio en la nube
echo "Iniciando servidor NATS.io (simulación nube)..."

# Verificar si Docker está corriendo
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker no está ejecutándose. Por favor inicia Docker primero."
    exit 1
fi

# Detener y remover contenedor existente si existe
docker stop nats-cloud-server 2>/dev/null || true
docker rm nats-cloud-server 2>/dev/null || true

# Ejecutar contenedor NATS con puertos diferentes para simular nube
docker run -d \
    --name nats-cloud-server \
    -p 5222:4222 \
    -p 9222:8222 \
    -p 7222:6222 \
    nats:latest \
    --jetstream \
    --http_port 8222 \
    --server_name "nats-cloud"

# Verificar que el contenedor esté corriendo
if [ $? -eq 0 ]; then
    echo "Servidor NATS (nube) iniciado exitosamente"
    echo "  - Puerto cliente: 5222"
    echo "  - Puerto monitoreo HTTP: 9222"
    echo "  - Puerto cluster: 7222"
    echo "  - JetStream habilitado"
    echo ""
    echo "Para conectarse: nats://localhost:5222"
    echo "Panel de monitoreo: http://localhost:9222"
else
    echo "Error al iniciar el servidor NATS (nube)"
    exit 1
fi