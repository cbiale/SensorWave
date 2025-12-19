# Funcionalidades Faltantes - SensorWave

Este documento lista las funcionalidades pendientes de implementación o corrección en el proyecto.

---

## ~~1. Inconsistencia de Prefijos S3~~ (RESUELTO)

**Estado**: Corregido  
**Archivos modificados**: `despachador/despachador.go`

Se unificó el prefijo a `nodos/<nodoID>.json` en ambos componentes:
- `cargarNodosDesdeS3()`: ahora usa prefijo `nodos/`
- `GuardarNodo()`: ahora usa key `nodos/<nodoID>.json`
- `EliminarNodo()`: ahora usa key `nodos/<nodoID>.json`

---

## ~~2. Handlers de Consultas no Implementados en Edge~~ (RESUELTO)

**Estado**: Corregido  
**Archivo modificado**: `edge/comunicacion_nube.go`

Se implementaron completamente los handlers de consultas:
- `iniciarListenersConsultas()`: Suscribe el edge a los tópicos de consulta
- `manejarConsultaRango()`: Procesa consultas de rango de tiempo
- `manejarConsultaUltimo()`: Procesa consultas del último punto
- `manejarConsultaPrimero()`: Procesa consultas del primer punto
- `enviarRespuesta()`: Serializa y publica respuestas usando Gob

Los handlers deserializan las solicitudes con `tipos.DeserializarGob()`, ejecutan las consultas locales y envían las respuestas al despachador.

---

## ~~3. Consulta de Datos Migrados a S3~~ (RESUELTO)

**Estado**: Corregido  
**Archivo modificado**: `despachador/despachador.go`

Se implementó la consulta híbrida S3 + Edge:
- `consultarDatosS3()`: Descarga y descomprime bloques de S3 en el rango especificado
- `listarBloquesEnRango()`: Lista bloques de S3 que intersectan con el rango de tiempo
- `descargarYDescomprimirBloque()`: Descarga un bloque de S3 y lo descomprime
- `consultarEdgeConTimeout()`: Consulta datos al edge con timeout
- `combinarResultados()`: Combina datos de S3 y edge, priorizando datos del edge en duplicados

**Flujo implementado**:
1. Usuario consulta serie X con rango de tiempo
2. Despachador consulta S3 y edge en paralelo
3. Si el edge no responde (timeout), se usan solo datos de S3
4. Se combinan resultados priorizando datos del edge (más recientes)

---

## ~~4. Migración Automática por Tiempo de Almacenamiento~~ (RESUELTO)

**Estado**: Corregido  
**Archivos modificados**: `tipos/nodo.go`, `edge/migracion_datos.go`

Se implementó la migración automática de datos basada en tiempo de almacenamiento:

**Cambios en `tipos/nodo.go`**:
- Agregado campo `TiempoAlmacenamiento int64` a struct `Serie`
- El campo representa el tiempo máximo en nanosegundos (0 = sin límite)

**Funciones implementadas en `edge/migracion_datos.go`**:
- `MigrarPorTiempoAlmacenamiento()`: Migra bloques que excedan el tiempo configurado para cada serie
- `parsearTiempoFinDeClave()`: Extrae el timestamp de fin de un bloque desde su clave
- `IniciarMigracionAutomatica(intervalo)`: Inicia un goroutine que ejecuta la migración periódicamente

**Flujo de migración**:
1. Recorre todas las series con `TiempoAlmacenamiento > 0`
2. Para cada serie, identifica bloques cuyo `tiempoFin` sea anterior al límite
3. Sube los bloques antiguos a S3
4. Elimina los bloques migrados de PebbleDB local

**Uso**:
```go
// Crear serie con tiempo de almacenamiento de 7 días
serie := tipos.Serie{
    Path:                 "dispositivo_001/temperatura",
    TipoDatos:            tipos.Real,
    TamañoBloque:         100,
    TiempoAlmacenamiento: 7 * 24 * time.Hour.Nanoseconds(), // 7 días
}

// Iniciar migración automática cada hora
manager.IniciarMigracionAutomatica(1 * time.Hour)
```

---

## ~~5. API Deprecada de AWS SDK~~ (RESUELTO)

**Estado**: Corregido  
**Archivos modificados**: `tipos/s3.go`, `despachador/despachador.go`, `edge/migracion_datos.go`, `test/edge/migracion_s3/migracion_s3_test.go`

Se reemplazó la API deprecada `aws.EndpointResolverWithOptionsFunc` por la API moderna usando `BaseEndpoint` en `s3.Options`.

**Solución implementada**:
- Se creó la función centralizada `tipos.CrearClienteS3()` que usa la API moderna
- Se eliminó código duplicado de creación de clientes S3 en todos los archivos
- Todos los archivos ahora usan `tipos.CrearClienteS3()` en lugar de código inline

```go
// Antes (deprecado):
customResolver := aws.EndpointResolverWithOptionsFunc(...)
awsCfg, err := config.LoadDefaultConfig(context.TODO(),
    config.WithEndpointResolverWithOptions(customResolver),
    ...
)
s3Client := s3.NewFromConfig(awsCfg, ...)

// Después (API moderna):
s3Client, err := tipos.CrearClienteS3(cfg)
```

**Beneficios**:
- Eliminación de warnings de API deprecada
- Código centralizado y más simple
- Un único punto de mantenimiento para configuración S3

---

## 6. Errores de Compilación en Tests del Middleware

**Prioridad**: Baja  
**Directorio afectado**: `test/middleware/`

Varios archivos tienen errores que impiden compilación:

### `test/middleware/latencia_case/mqtt_solo.go`
- Redeclaración de `main` en el mismo paquete
- Tipo incorrecto en parámetros de función

### `test/middleware/ejemplo_case/`
- Múltiples archivos `main.go` en subdirectorios separados (correcto)
- Pero algunos archivos sueltos tienen conflictos

**Nota**: Estos son tests de prueba/benchmarking, no afectan la funcionalidad principal.

---

## ~~7. Código no Utilizado~~ (RESUELTO)

**Estado**: Corregido  
**Archivo modificado**: `edge/reglas.go`

Se eliminó el método `contarDatosEnCache` que no se usaba.

---

## Resumen por Prioridad

| Prioridad | Ítem | Estado |
|-----------|------|--------|
| ~~Alta~~ | ~~Inconsistencia prefijos S3~~ | RESUELTO |
| ~~Alta~~ | ~~Handlers consultas en edge~~ | RESUELTO |
| ~~Media~~ | ~~Consulta datos en S3~~ | RESUELTO |
| ~~Baja~~ | ~~Migración por tiempo~~ | RESUELTO |
| ~~Baja~~ | ~~API deprecada AWS~~ | RESUELTO |
| Baja | Tests middleware | Pendiente |
| ~~Muy baja~~ | ~~Código no usado~~ | RESUELTO |

---

## Notas

- Todas las funcionalidades de prioridad "Alta", "Media" y "Baja" han sido implementadas
- Solo queda pendiente el ítem de "Tests middleware" (prioridad Baja)
- El sistema edge funciona correctamente de forma autónoma (sin despachador)
- El sistema distribuido (edge + despachador) está completamente funcional
- La migración automática por tiempo de almacenamiento permite configurar retención de datos por serie
