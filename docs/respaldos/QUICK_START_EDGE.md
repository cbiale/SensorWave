# Quick Start Guide - Edge Module

## Edge Module - Two Operating Modes

### Mode 1: Local (Standalone)

Use this mode when the edge node operates independently without cloud registration.

```go
import "github.com/cbiale/sensorwave/edge"

// Create edge manager in local mode
manager, err := edge.Crear("edge.db", "", "8080")
if err != nil {
    log.Fatal(err)
}
defer manager.Cerrar()

// Create a series
serie := tipos.Serie{
    Path:             "sensor/temperatura",
    TipoDatos:        tipos.Real,
    TamañoBloque:     100,
    CompresionBloque: tipos.Snappy,
    CompresionBytes:  tipos.Xor,
    Tags:             map[string]string{"location": "room1"},
}

err = manager.CrearSerie(serie)
if err != nil {
    log.Fatal(err)
}

// Insert data
err = manager.Insertar("sensor/temperatura", time.Now().Unix(), 23.5)
```

**Features**:
- ✅ Local PebbleDB storage
- ✅ Works offline
- ✅ Motor de reglas
- ❌ No cloud registration
- ❌ No despachador discovery

### Mode 2: Cloud-Connected

Use this mode when the edge node should register in Garage for despachador discovery.

```go
import "github.com/cbiale/sensorwave/edge"

// Garage configuration JSON
configGarage := `{
    "endpoint": "http://localhost:3900",
    "access_key_id": "GK31c2f218a2e44f485b94239e",
    "secret_access_key": "b892c0665f5ada1092f5df1f66b5b9d23b95a09e024e7c5b",
    "bucket": "nodos",
    "region": "garage"
}`

// Create edge manager in cloud mode
manager, err := edge.Crear("edge.db", configGarage, "8080")
if err != nil {
    log.Fatal(err)
}
defer manager.Cerrar()

// Create series (automatically updates registration in Garage)
serie := tipos.Serie{
    Path:             "sensor/temperatura",
    TipoDatos:        tipos.Real,
    TamañoBloque:     100,
    CompresionBloque: tipos.Snappy,
    CompresionBytes:  tipos.Xor,
}

err = manager.CrearSerie(serie)
// This automatically calls RegistrarEnGarage() internally
```

**Features**:
- ✅ Local PebbleDB storage
- ✅ Works offline (queues updates)
- ✅ Motor de reglas
- ✅ Cloud registration in Garage
- ✅ Despachador discovery
- ✅ Data migration to Garage

### Data Migration to Garage

Migrate old data from PebbleDB to Garage:

```go
// Must configure Garage first
err = manager.ConfigurarGarage(ConfiguracionGarage{
    Endpoint:        "http://localhost:3900",
    AccessKeyID:     "GK...",
    SecretAccessKey: "...",
    Bucket:          "sensorwave-data",
    Region:          "garage",
})

// Migrate data
err = manager.Migrar()
if err != nil {
    log.Fatal(err)
}
```

### Getting Garage Credentials

```bash
# Start Garage
bash contenedores/iniciar_garage.sh

# Create access key
docker exec -ti garage garage key new --name my-edge-node

# Output:
# Access Key ID: GK31c2f218a2e44f485b94239e
# Secret Access Key: b892c0665f5ada1092f5df1f66b5b9d23b95a09e024e7c5b
```

### Registration Format in Garage

When edge registers, it creates an object at `nodos/<nodoID>.json`:

```json
{
    "nodo_id": "edge-550e8400-e29b-41d4-a716-446655440000",
    "direccion_ip": "192.168.1.100",
    "puerto_http": "8080",
    "series": {
        "sensor/temperatura": {
            "path": "sensor/temperatura",
            "serie_id": 1,
            "tipo_datos": "Real",
            "tamaño_bloque": 100,
            "compresion_bloque": "Snappy",
            "compresion_bytes": "Xor",
            "tags": {"location": "room1"}
        }
    }
}
```

### Registration Updates

The edge node updates its registration in Garage:
- ✅ On creation (if Garage configured)
- ✅ When new series are added
- ❌ No periodic heartbeats (on-demand only)

### Comparison: Old vs New

| Feature | Old (FDB) | New (Garage) |
|---------|-----------|--------------|
| Node registration | FoundationDB | Garage (S3) |
| Heartbeats | Every 30s | None (on-demand) |
| Registration trigger | Startup + timer | Startup + new series |
| Data migration | Garage | Garage |
| Despachador reads from | Garage | Garage ✅ |
| Consistency | ❌ Inconsistent | ✅ Consistent |
| Dependencies | FDB + Garage | Garage only |

### Testing

```bash
# Compile edge
go build ./edge/...

# Run integration test
cd test/edge/integrador
go build
./integrador
```

### Migration Checklist

If migrating from old FDB-based edge code:

- [x] Remove FoundationDB setup code
- [x] Update `edge.Crear()` calls (3 params instead of 4)
- [x] Change second parameter from FDB cluster to Garage JSON config
- [x] Use empty string "" for local mode
- [x] Remove heartbeat monitoring (not needed anymore)
- [x] Update import paths if needed

### Example: Complete Migration

**Before (FDB)**:
```go
manager, err := edge.Crear("edge.db", "localhost:4500", "8080")
```

**After (Local mode)**:
```go
manager, err := edge.Crear("edge.db", "", "8080")
```

**After (Cloud mode)**:
```go
configJSON := `{"endpoint":"http://localhost:3900","access_key_id":"...","secret_access_key":"...","bucket":"nodos","region":"garage"}`
manager, err := edge.Crear("edge.db", configJSON, "8080")
```

---

For more details see:
- `EDGE_MIGRATION_SUMMARY.md` - Complete migration details
- `RESUMEN_REFACTORIZACION.md` - Full refactoring summary
- `README.md` - General project documentation
