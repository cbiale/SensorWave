# EdgeSensorWave Admin v1.0.5

Una interfaz web completa para administrar y monitorear bases de datos EdgeSensorWave.

## 🚀 Características

### Dashboard en Tiempo Real
- **Métricas del sistema** actualizadas automáticamente cada 10 segundos
- **Estado de sensores** con indicadores visuales (online/warning/offline)
- **Gráficos interactivos** usando Chart.js para visualización de datos
- **Auto-refresh configurable** con controles de usuario

### Query Builder Visual
- **Constructor visual de consultas** sin necesidad de escribir código
- **Autocompletado de sensores** para facilitar la selección
- **Rangos rápidos** (1h, 6h, 24h, 7d, 30d)
- **Vista previa de consultas** en formato SQL-like
- **Resultados en tabla y gráficos** intercambiables
- **Consultas guardadas** para reutilización

### Sistema de Alertas
- **Configuración visual** de reglas de alerta
- **Operadores flexibles** (>, <, >=, <=, ==, !=)
- **Monitoreo automático** cada 30 segundos
- **Historial de eventos** con resolución manual
- **Habilitación/deshabilitación** de alertas individualmente

### Exportación de Datos
- **Formatos múltiples**: CSV y JSON
- **Opciones configurables**: calidad, metadatos, zona horaria
- **Plantillas predefinidas** para casos comunes
- **Vista previa estimada** de registros y tamaño
- **Historial de exportaciones** con gestión local

### Gestión de Mantenimiento
- **Compactación manual** de base de datos
- **Sincronización forzada** al disco
- **Limpieza de datos antiguos** según políticas de retención
- **Respaldos completos** descargables
- **Análisis de almacenamiento** con gráficos detallados
- **Registro de operaciones** en tiempo real

## 🛠️ Instalación y Uso

### Compilación
```bash
# Desde el directorio edge/
go build -o edgesensorwave-admin cmd/admin/main.go
```

### Ejecución
```bash
# Básico
./edgesensorwave-admin

# Con configuración personalizada
./edgesensorwave-admin --db-path=/path/to/sensores.esw --port=8080 --host=0.0.0.0

# Modo desarrollo (recarga automática de templates)
./edgesensorwave-admin --dev
```

### Parámetros
- `--db-path`: Ruta a la base de datos EdgeSensorWave (default: "./sensores.esw")
- `--host`: Host del servidor web (default: "localhost")
- `--port`: Puerto del servidor web (default: "8080")
- `--dev`: Modo desarrollo con recarga automática

### Acceso Web
Una vez iniciado, accede a: `http://localhost:8080`

## 🏗️ Arquitectura Técnica

### Stack Tecnológico
- **Backend**: Go con net/http nativo
- **Frontend**: HTML5 + htmx para interactividad
- **Gráficos**: Chart.js para visualizaciones
- **Styling**: CSS moderno con variables CSS y Flexbox/Grid
- **Deployment**: Binario único autosuficiente

### Estructura del Proyecto
```
cmd/admin/
├── main.go                 # Punto de entrada del servidor

internal/admin/
├── server.go              # Servidor HTTP principal
├── handlers.go            # Manejadores de rutas
├── alerts.go              # Sistema de alertas
└── export.go              # Funciones de exportación

web/
├── templates/
│   ├── base.html          # Template base
│   ├── dashboard.html     # Dashboard principal
│   ├── query.html         # Constructor de consultas
│   ├── alerts.html        # Gestión de alertas
│   ├── export.html        # Exportación de datos
│   └── maintenance.html   # Mantenimiento del sistema
└── static/
    ├── css/style.css      # Estilos principales
    └── js/app.js          # JavaScript de la aplicación
```

### API Endpoints

#### Páginas Web
- `GET /` - Dashboard principal
- `GET /consulta` - Constructor de consultas
- `GET /alertas` - Gestión de alertas
- `GET /exportar` - Exportación de datos
- `GET /mantenimiento` - Mantenimiento del sistema

#### API REST
- `GET /api/metrics` - Métricas del sistema
- `GET /api/sensors` - Estado de sensores
- `POST /api/query/execute` - Ejecutar consulta
- `GET|POST /api/alerts` - Gestión de alertas
- `POST /api/export/csv` - Exportar CSV
- `POST /api/export/json` - Exportar JSON
- `POST /api/maintenance/compact` - Compactar DB
- `GET /api/maintenance/stats` - Estadísticas de mantenimiento

## 🔧 Configuración Avanzada

### Auto-refresh
El dashboard se actualiza automáticamente cada 10 segundos. Esto puede controlarse:
- Checkbox "Auto-refresh" en el dashboard
- Variable JavaScript `EdgeAdmin.config.refreshInterval`

### Notificaciones
Sistema de notificaciones integrado con control de límites:
- Máximo 5 notificaciones simultáneas
- Auto-eliminación después de 5 segundos
- Cierre manual disponible

### Almacenamiento Local
Usa localStorage para:
- Consultas guardadas del usuario
- Historial de exportaciones
- Configuraciones de usuario
- Estado de primera visita

### Atajos de Teclado
- `Ctrl+R` / `F5`: Actualizar página
- `Ctrl+/`: Mostrar ayuda de atajos
- `Ctrl+1-5`: Navegación rápida entre secciones

## 📊 Características de Rendimiento

### Optimizaciones
- **Lazy loading** de datos grandes
- **Compresión automática** de respuestas HTTP
- **Cache de templates** en memoria
- **Pooling de conexiones** a la base de datos
- **Auto-pausa** de refresh cuando la página no es visible

### Métricas de Rendimiento
- Tiempo de carga inicial: < 2 segundos
- Actualización de dashboard: < 500ms
- Exportaciones: Optimizadas para datasets grandes
- Memoria del servidor: < 50MB en uso normal

## 🛡️ Seguridad

### Consideraciones
- **Solo HTTP local**: Diseñado para uso en localhost o redes internas
- **Sin autenticación**: Para simplicidad en edge computing
- **Validación de entradas**: Sanitización de parámetros de consulta
- **CORS habilitado**: Para APIs, restringible si es necesario

### Hardening para Producción
Si se usa en producción, considerar:
- Proxy reverso con HTTPS (nginx, Apache)
- Autenticación básica a nivel de proxy
- Firewall para restringir acceso a IPs específicas
- Monitoreo de logs de acceso

## 🐛 Resolución de Problemas

### Problemas Comunes

#### Error: "Templates not loaded"
```bash
# Solución: Verificar que el directorio web/templates existe
ls web/templates/
```

#### Error: "Base de datos no encontrada"
```bash
# Crear base de datos de prueba primero
go run ejemplos/basico/main.go
```

#### Puerto ya en uso
```bash
# Cambiar puerto
./edgesensorwave-admin --port=8081
```

#### Archivos estáticos no cargan
```bash
# Verificar estructura de directorios
ls web/static/css/
ls web/static/js/
```

### Logs de Debugging
El servidor muestra logs de:
- Inicialización del servidor
- Peticiones HTTP (`método URL IP`)
- Errores de templates
- Operaciones de base de datos

### Modo Desarrollo
```bash
./edgesensorwave-admin --dev
```
- Recarga automática de templates
- Logs más detallados
- Sin cache de archivos estáticos

## 📈 Roadmap

### Versión 1.1.0
- [ ] Autenticación opcional (JWT)
- [ ] Themes (dark/light mode)
- [ ] Exportación programada
- [ ] Webhooks para alertas
- [ ] API de configuración remota

### Versión 1.2.0
- [ ] Multi-tenancy
- [ ] Dashboards personalizables
- [ ] Plugins de visualización
- [ ] Integración con Grafana
- [ ] Métricas de Prometheus

## 🤝 Contribución

Para contribuir al administrador web:

1. Hacer cambios en `web/templates/` para UI
2. Modificar `internal/admin/` para lógica backend
3. Actualizar `web/static/` para estilos/JavaScript
4. Probar con `--dev` para desarrollo
5. Compilar y probar versión final

El administrador está diseñado para ser:
- **Liviano**: Un solo binario
- **Responsive**: Funciona en móviles y desktop
- **Intuitivo**: UX optimizada para administradores de sistemas
- **Extensible**: Fácil agregar nuevas características

---

**EdgeSensorWave Admin v1.0.5** - Administración web moderna para bases de datos IoT edge ⚡