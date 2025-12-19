#!/bin/bash

# Script para iniciar Garage (alternativa S3) con soporte de replicación
echo "Iniciando servidor Garage (S3)..."

# Verificar si Docker está corriendo
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker no está ejecutándose. Por favor inicia Docker primero."
    exit 1
fi

# Crear directorio para datos de Garage si no existe
GARAGE_DATA="$HOME/.garage/data"
GARAGE_META="$HOME/.garage/meta"
mkdir -p "$GARAGE_DATA"
mkdir -p "$GARAGE_META"

# Detener y remover contenedor existente si existe
docker stop garage 2>/dev/null || true
docker rm garage 2>/dev/null || true

# Ejecutar contenedor Garage
docker run -d \
    --name garage \
    -p 3900:3900 \
    -p 3901:3901 \
    -p 3902:3902 \
    -v "$GARAGE_DATA:/data" \
    -v "$GARAGE_META:/meta" \
    -e RUST_LOG=garage=info \
    dxflrs/garage:latest \
    server

# Esperar a que el servicio inicie
echo "Esperando a que Garage inicie..."
sleep 3

# Verificar que el contenedor esté corriendo
if [ $? -eq 0 ] && docker ps | grep -q garage; then
    echo "Servidor Garage iniciado exitosamente"
    echo "  - Puerto API S3: 3900"
    echo "  - Puerto Admin API: 3901"
    echo "  - Puerto Web: 3902"
    echo "  - Replicación: Habilitada (modo cluster)"
    echo ""
    echo "Configuración inicial:"
    echo "  1. Endpoint S3: http://localhost:3900"
    echo "  2. Admin API: http://localhost:3901"
    echo ""
    echo "Para configurar el cluster y crear credenciales, ejecutar:"
    echo "  docker exec -ti garage garage status"
    echo "  docker exec -ti garage garage key new --name mi-aplicacion"
    echo "  docker exec -ti garage garage bucket create mi-bucket"
else
    echo "Error al iniciar el servidor Garage"
    exit 1
fi
