package motor

import (
	"time"
)

// Opciones contiene la configuración para la base de datos EdgeSensorWave
type Opciones struct {
	// Configuración de memoria
	CacheBytes     int64 // Tamaño del cache en bytes
	MemTableBytes  int64 // Tamaño del MemTable en bytes
	MaxOpenFiles   int   // Máximo número de archivos abiertos

	// Configuración de compactación
	CompactacionNivelMinimo int // Nivel mínimo para compactación automática
	CompactacionParalela    bool // Permitir compactación en paralelo

	// Configuración de escritura
	TamañoLoteEscritura int   // Tamaño del lote para escrituras
	SincronizarEscritura bool // Sincronizar cada escritura al disco

	// Configuración de retención
	TiempoRetencion time.Duration // Tiempo de retención de datos
	AutoCompactar   bool          // Compactación automática habilitada

	// Configuración de validación
	ValidarIDs     bool // Validar IDs de sensores
	ValidarRangos  bool // Validar rangos de valores

	// Configuración de logging
	LogNivel string // Nivel de logging (debug, info, warn, error)
	LogRuta  string // Ruta del archivo de log
}

// OpcionesDefecto retorna la configuración por defecto optimizada para edge IoT
func OpcionesDefecto() *Opciones {
	return &Opciones{
		// Configuración de memoria optimizada para edge (< 50MB)
		CacheBytes:    16 * 1024 * 1024, // 16MB cache
		MemTableBytes: 8 * 1024 * 1024,  // 8MB MemTable
		MaxOpenFiles:  100,

		// Compactación no-blocking
		CompactacionNivelMinimo: 2,
		CompactacionParalela:    false, // Evitar overhead en edge

		// Escritura optimizada para 100K writes/sec
		TamañoLoteEscritura:  1000,
		SincronizarEscritura: false, // Para rendimiento en edge

		// Retención por defecto de 30 días
		TiempoRetencion: 30 * 24 * time.Hour,
		AutoCompactar:   true,

		// Validación habilitada por defecto
		ValidarIDs:    true,
		ValidarRangos: true,

		// Logging mínimo para edge
		LogNivel: "warn",
		LogRuta:  "",
	}
}

// OpcionesRendimiento retorna configuración optimizada para máximo rendimiento
func OpcionesRendimiento() *Opciones {
	opts := OpcionesDefecto()
	
	// Aumentar memoria para rendimiento
	opts.CacheBytes = 32 * 1024 * 1024     // 32MB cache
	opts.MemTableBytes = 16 * 1024 * 1024  // 16MB MemTable
	opts.MaxOpenFiles = 200

	// Lotes más grandes para mejor throughput
	opts.TamañoLoteEscritura = 5000

	// Desactivar validaciones para velocidad
	opts.ValidarIDs = false
	opts.ValidarRangos = false

	// Logging mínimo
	opts.LogNivel = "error"

	return opts
}

// OpcionesMemoriaMinima retorna configuración para footprint mínimo de memoria
func OpcionesMemoriaMinima() *Opciones {
	opts := OpcionesDefecto()
	
	// Memoria muy limitada
	opts.CacheBytes = 4 * 1024 * 1024      // 4MB cache
	opts.MemTableBytes = 2 * 1024 * 1024   // 2MB MemTable
	opts.MaxOpenFiles = 50

	// Lotes pequeños
	opts.TamañoLoteEscritura = 100

	// Sin logging para ahorrar memoria
	opts.LogNivel = "error"

	return opts
}

// Validar verifica que las opciones sean válidas
func (o *Opciones) Validar() error {
	if o.CacheBytes <= 0 {
		o.CacheBytes = 16 * 1024 * 1024
	}
	
	if o.MemTableBytes <= 0 {
		o.MemTableBytes = 8 * 1024 * 1024
	}

	if o.MaxOpenFiles <= 0 {
		o.MaxOpenFiles = 100
	}

	if o.TamañoLoteEscritura <= 0 {
		o.TamañoLoteEscritura = 1000
	}

	if o.TiempoRetencion <= 0 {
		o.TiempoRetencion = 30 * 24 * time.Hour
	}

	return nil
}