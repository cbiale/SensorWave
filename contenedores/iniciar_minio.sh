#!/bin/bash

# Script para iniciar el servidor MinIO
echo "Iniciando servidor MinIO..."

# Verificar si Docker está corriendo
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker no está ejecutándose. Por favor inicia Docker primero."
    exit 1
fi

# Detener y remover contenedor existente si existe
docker stop minio1 2>/dev/null || true
docker rm minio1 2>/dev/null || true

# Ejecutar contenedor MinIO
docker run -d \
    --name minio1 \
    -p 9000:9000 \
    -p 9001:9001 \
    -e "MINIO_ACCESS_KEY=miniominio" \
    -e "MINIO_SECRET_KEY=miniominio" \
    minio/minio server /data --console-address ":9001" --address ":9000"

# Verificar que el contenedor esté corriendo
if [ $? -eq 0 ]; then
    echo "Servidor MinIO iniciado exitosamente"
    echo "  - Puerto API: 9000"
    echo "  - Puerto consola: 9001"
    echo "  - Access Key: miniominio"
    echo "  - Secret Key: miniominio"
    echo ""
    echo "Para conectarse: http://localhost:9000"
    echo "Consola web: http://localhost:9001"
else
    echo "Error al iniciar el servidor MinIO"
    exit 1
fi
