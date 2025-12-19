# Migración de datos a Garage (S3)

Este ejemplo demuestra cómo migrar datos desde PebbleDB (almacenamiento local en el edge) a Garage (almacenamiento S3 compatible) para respaldo y replicación.

## Prerrequisitos

1. Tener Garage ejecutándose:
```bash
cd ../../..
./contenedores/iniciar_garage.sh
```

2. Crear credenciales de acceso en Garage:
```bash
# Crear una clave de acceso
docker exec -ti garage garage key new --name sensorwave

# Esto devolverá algo como:
# Key:
#   Key ID: GK31c2f218a2e44f485b94239e
#   Secret: 4420ec9e620156a04d42d6e78e7c14b2
```

3. Crear un bucket para almacenar los datos:
```bash
# Crear el bucket
docker exec -ti garage garage bucket create sensorwave-data

# Vincular el bucket con la clave
docker exec -ti garage garage bucket allow --read --write sensorwave-data --key GK31c2f218a2e44f485b94239e
```

## Configuración

### Opción 1: Variables de entorno

```bash
export GARAGE_ACCESS_KEY="GK31c2f218a2e44f485b94239e"
export GARAGE_SECRET_KEY="4420ec9e620156a04d42d6e78e7c14b2"
export GARAGE_ENDPOINT="http://localhost:3900"
export GARAGE_BUCKET="sensorwave-data"
export GARAGE_REGION="garage"
```

### Opción 2: Configuración programática

```go
cfg := edge.ConfiguracionGarage{
    Endpoint:        "http://localhost:3900",
    AccessKeyID:     "GK31c2f218a2e44f485b94239e",
    SecretAccessKey: "4420ec9e620156a04d42d6e78e7c14b2",
    Bucket:          "sensorwave-data",
    Region:          "garage",
}

err := manager.ConfigurarGarage(cfg)
```

## Uso

### Ejecutar el ejemplo:

```bash
# Compilar
go build -o migracion_garage main.go

# Ejecutar
./migracion_garage
```

### Uso en código propio:

```go
package main

import (
    "log"
    "github.com/cbiale/sensorwave/edge"
)

func main() {
    // Crear manager edge
    manager, err := edge.Crear("./mi_db", "", "8080")
    if err != nil {
        log.Fatal(err)
    }
    defer manager.Cerrar()

    // Configurar Garage
    cfg := edge.ConfiguracionGarage{
        Endpoint:        "http://localhost:3900",
        AccessKeyID:     "tu_access_key",
        SecretAccessKey: "tu_secret_key",
        Bucket:          "sensorwave-data",
        Region:          "garage",
    }
    
    err = manager.ConfigurarGarage(cfg)
    if err != nil {
        log.Fatal(err)
    }

    // Migrar datos
    err = manager.Migrar()
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Migración completada!")
}
```

## Estructura de datos en Garage

Los datos se almacenan en Garage con la siguiente estructura:

```
bucket/
  nodoID/
    data/
      serie_1_timestamp_inicio_timestamp_fin
      serie_2_timestamp_inicio_timestamp_fin
      ...
```

Por ejemplo:
```
sensorwave-data/
  edge-node-123/
    data/serie1/1234567890/1234567900
    data/serie1/1234567901/1234567910
    data/serie2/1234567890/1234567900
```

## Características

- **Migración incremental**: Solo migra datos que no han sido migrados previamente
- **Preservación de estructura**: Mantiene la estructura de claves de PebbleDB
- **Limpieza automática**: Elimina datos de PebbleDB después de migrarlos exitosamente
- **Compatibilidad S3**: Funciona con cualquier servicio compatible con S3 (AWS S3, MinIO, Garage, etc.)
- **Replicación**: Garage maneja automáticamente la replicación entre nodos

## Verificar datos migrados

```bash
# Listar objetos en el bucket
docker exec -ti garage garage bucket info sensorwave-data

# Usando la CLI de AWS (si está instalada)
aws s3 --endpoint-url http://localhost:3900 ls s3://sensorwave-data/
```

## Notas importantes

1. La migración **elimina** los datos de PebbleDB después de subirlos a Garage
2. Asegúrate de tener suficiente espacio en Garage para todos los datos
3. La migración se realiza de forma síncrona (bloqueante)
4. Se recomienda ejecutar la migración cuando el sistema no esté bajo carga
5. Los logs muestran el progreso cada 100 registros migrados
