#!/bin/bash

# Script para iniciar una instancia de FoundationDB
echo "Iniciando servidor FoundationDB..."

# Verificar si Docker está corriendo
if ! docker info > /dev/null 2>&1; then
    echo "Error: Docker no está ejecutándose. Por favor inicia Docker primero."
    exit 1
fi

# Configuración por defecto
FDB_VERSION=${FDB_VERSION:-"7.3"}
FDB_CLUSTER_FILE=${FDB_CLUSTER_FILE:-"/tmp/fdb.cluster"}
FDB_DATA_DIR=${FDB_DATA_DIR:-"/tmp/fdb_data"}
FDB_PORT=${FDB_PORT:-"4500"}
FDB_MONITOR_PORT=${FDB_MONITOR_PORT:-"8900"}

# Crear directorio de datos si no existe
mkdir -p $FDB_DATA_DIR

# Detener y remover contenedor existente si existe
docker stop fdb-server 2>/dev/null || true
docker rm fdb-server 2>/dev/null || true

# Crear archivo de cluster para FoundationDB
echo "Creando archivo de cluster..."
cat > $FDB_CLUSTER_FILE << EOF
foundationdb:description:FoundationDB cluster configuration file
foundationdb:version:7.3

foundationdb:process:
  - class: stateless
    listen_address: 0.0.0.0:$FDB_PORT
    public_address: 127.0.0.1:$FDB_PORT
    log_group: default
    logdir: $FDB_DATA_DIR/logs
    datadir: $FDB_DATA_DIR/data
    command_line: --knob_disable_posix_kernel_aio=1

foundationdb:general:
  cluster_file_path: $FDB_CLUSTER_FILE
  restart_on_error: true

foundationdb:backup_agent:
  - start_delay: 5
    log_group: default
    logdir: $FDB_DATA_DIR/logs
EOF

# Ejecutar contenedor FoundationDB
echo "Iniciando contenedor FoundationDB..."
docker run -d \
    --name fdb-server \
    -p $FDB_PORT:$FDB_PORT \
    -p $FDB_MONITOR_PORT:$FDB_MONITOR_PORT \
    -v $FDB_CLUSTER_FILE:/etc/foundationdb/fdb.cluster \
    -v $FDB_DATA_DIR:/var/foundationdb/data \
    foundationdb/foundationdb:$FDB_VERSION \
    /usr/bin/fdbserver \
    --cluster_file /etc/foundationdb/fdb.cluster \
    --public_address 127.0.0.1:$FDB_PORT \
    --listen_address 0.0.0.0:$FDB_PORT \
    --datadir /var/foundationdb/data/data \
    --logdir /var/foundationdb/data/logs \
    --knob_disable_posix_kernel_aio=1

# Esperar a que FoundationDB esté disponible
echo "Esperando a que FoundationDB esté disponible..."
sleep 10

# Verificar que el contenedor esté corriendo
if [ $? -eq 0 ]; then
    echo "Servidor FoundationDB iniciado exitosamente"
    echo "  - Puerto cliente: $FDB_PORT"
    echo "  - Puerto monitoreo: $FDB_MONITOR_PORT"
    echo "  - Archivo cluster: $FDB_CLUSTER_FILE"
    echo "  - Directorio datos: $FDB_DATA_DIR"
    echo ""
    echo "Para conectarse:"
    echo "  fdbcli --exec 'status; getversion;'"
    echo ""
    echo "Variables de entorno útiles:"
    echo "  export FDB_CLUSTER_FILE=$FDB_CLUSTER_FILE"
    echo "  export FDB_COORDINATOR=127.0.0.1:$FDB_PORT"
else
    echo "Error al iniciar el servidor FoundationDB"
    exit 1
fi

# Esperar adicionalmente para que el cluster esté completamente listo
echo "Verificando estado del cluster..."
sleep 5

# Verificar el estado del cluster
docker exec fdb-server fdbcli --exec "status; getversion;" 2>/dev/null || {
    echo "Advertencia: FoundationDB está iniciando pero aún no está completamente disponible."
    echo "Espere unos segundos más y ejecute:"
    echo "  docker exec fdb-server fdbcli --exec 'status; getversion;'"
}

echo ""
echo "Comandos útiles:"
echo "  Ver estado: docker exec fdb-server fdbcli --exec 'status;'"
echo "  Ver versión: docker exec fdb-server fdbcli --exec 'getversion;'"
echo "  Ver logs: docker logs fdb-server"
echo "  Detener: docker stop fdb-server"
echo ""
echo "Para usar en Go:"
echo "  import \"github.com/apple/foundationdb/bindings/go/src/fdb\""
echo "  db, err := fdb.OpenDatabase(\"$FDB_CLUSTER_FILE\")"