package pebble_backend

import (
	"fmt"
	"time"

	"github.com/cockroachdb/pebble"
	"edgesensorwave/pkg/motor"
)

// Lote implementa operaciones de lote usando pebble.Batch
type Lote struct {
	batch    *pebble.Batch
	db       *DB
	tamaño   int
	cerrado  bool
}

// NuevoLote crea un nuevo lote para operaciones masivas
func (db *DB) NuevoLote() motor.Lote {
	return &Lote{
		batch: db.pb.NewBatch(),
		db:    db,
	}
}

// Agregar agrega una entrada al lote con calidad por defecto (Buena)
func (l *Lote) Agregar(idSensor string, valor float64, timestamp time.Time) error {
	return l.AgregarConCalidad(idSensor, valor, motor.CalidadBuena, timestamp, nil)
}

// AgregarConCalidad agrega una entrada al lote con calidad y metadatos específicos
func (l *Lote) AgregarConCalidad(idSensor string, valor float64, calidad motor.CalidadDato, timestamp time.Time, metadatos map[string]string) error {
	if l.cerrado {
		return fmt.Errorf("lote ya ha sido cerrado")
	}
	
	// Validar entrada si está habilitado
	if l.db.opciones.ValidarIDs {
		if err := validarIDSensor(idSensor); err != nil {
			return fmt.Errorf("ID de sensor inválido: %w", err)
		}
	}
	
	// Crear clave y valor
	clave := &motor.ClaveSensor{
		IDSensor:  idSensor,
		Timestamp: timestamp,
	}
	
	valorSensor := &motor.ValorSensor{
		Valor:     valor,
		Calidad:   calidad,
		Metadatos: metadatos,
	}
	
	// Serializar
	claveBytes := SerializarClave(clave)
	valorBytes, err := SerializarValor(valorSensor)
	if err != nil {
		return fmt.Errorf("error serializando valor: %w", err)
	}
	
	// Agregar al lote
	if err := l.batch.Set(claveBytes, valorBytes, nil); err != nil {
		return fmt.Errorf("error agregando al lote: %w", err)
	}
	
	l.tamaño++
	return nil
}

// Tamaño retorna el número de entradas en el lote
func (l *Lote) Tamaño() int {
	return l.tamaño
}

// Limpiar limpia el lote sin escribir las entradas
func (l *Lote) Limpiar() {
	if !l.cerrado {
		l.batch.Reset()
		l.tamaño = 0
	}
}

// ConfirmarLote confirma y aplica todas las operaciones del lote a la base de datos
func (db *DB) ConfirmarLote(lote motor.Lote) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	
	if db.cerrada {
		return fmt.Errorf("base de datos cerrada")
	}
	
	// Cast al tipo concreto
	l, ok := lote.(*Lote)
	if !ok {
		return fmt.Errorf("tipo de lote inválido")
	}
	
	if l.cerrado {
		return fmt.Errorf("lote ya ha sido confirmado o cerrado")
	}
	
	if l.tamaño == 0 {
		l.cerrado = true
		return nil // Lote vacío, no hay nada que confirmar
	}
	
	// Aplicar el lote
	writeOpts := pebble.WriteOptions{}
	if db.opciones.SincronizarEscritura {
		writeOpts.Sync = true
	}
	
	if err := db.pb.Apply(l.batch, &writeOpts); err != nil {
		return fmt.Errorf("error aplicando lote: %w", err)
	}
	
	// Actualizar estadísticas
	// Nota: esto es una aproximación, en un entorno de alta concurrencia
	// sería mejor usar atomic operations
	db.contadorRegistros += int64(l.tamaño)
	
	// Marcar lote como cerrado
	l.cerrado = true
	
	return nil
}

// aplicarLoteConRetentacion aplica un lote y gestiona retención de datos
func (db *DB) aplicarLoteConRetentacion(lote *Lote) error {
	// Primero aplicar el lote
	if err := db.ConfirmarLote(lote); err != nil {
		return err
	}
	
	// Si la retención está habilitada, limpiar datos antiguos
	if db.opciones.TiempoRetencion > 0 {
		return db.limpiarDatosAntiguos()
	}
	
	return nil
}

// limpiarDatosAntiguos elimina datos más antiguos que el tiempo de retención
func (db *DB) limpiarDatosAntiguos() error {
	if db.opciones.TiempoRetencion <= 0 {
		return nil
	}
	
	// Calcular timestamp límite
	limite := time.Now().Add(-db.opciones.TiempoRetencion)
	
	// Crear iterador para encontrar claves antiguas
	iter, err := db.pb.NewIter(&pebble.IterOptions{
		LowerBound: []byte{PrefijoSensor},
		UpperBound: []byte{PrefijoSensor + 1},
	})
	if err != nil {
		return fmt.Errorf("error creando iterador para limpieza: %w", err)
	}
	defer iter.Close()
	
	// Crear lote para eliminaciones
	deleteBatch := db.pb.NewBatch()
	deletions := 0
	maxDeletions := 1000 // Limitar eliminaciones por lote para evitar overhead
	
	for iter.First(); iter.Valid() && deletions < maxDeletions; iter.Next() {
		// Deserializar clave para obtener timestamp
		clave, err := DeserializarClave(iter.Key())
		if err != nil {
			continue // Saltar claves inválidas
		}
		
		// Si el timestamp es anterior al límite, eliminar
		if clave.Timestamp.Before(limite) {
			if err := deleteBatch.Delete(iter.Key(), nil); err != nil {
				return fmt.Errorf("error agregando eliminación al lote: %w", err)
			}
			deletions++
		} else {
			// Como las claves están ordenadas por tiempo, ya no hay más claves antiguas
			break
		}
	}
	
	if err := iter.Error(); err != nil {
		return fmt.Errorf("error iterando para limpieza: %w", err)
	}
	
	// Aplicar eliminaciones si hay alguna
	if deletions > 0 {
		if err := db.pb.Apply(deleteBatch, &pebble.WriteOptions{}); err != nil {
			return fmt.Errorf("error aplicando eliminaciones: %w", err)
		}
		
		// Actualizar contador
		db.contadorRegistros -= int64(deletions)
	}
	
	return nil
}

// LoteOptimizado crea un lote optimizado para inserción masiva
func (db *DB) LoteOptimizado(capacidad int) motor.Lote {
	// Crear lote con capacidad estimada para evitar reallocations
	batch := db.pb.NewBatch()
	
	return &Lote{
		batch: batch,
		db:    db,
	}
}

// ConfirmarLoteConEstadisticas confirma un lote y actualiza estadísticas detalladas
func (db *DB) ConfirmarLoteConEstadisticas(lote motor.Lote) (*LoteEstadisticas, error) {
	inicio := time.Now()
	
	// Obtener estadísticas del lote
	l, ok := lote.(*Lote)
	if !ok {
		return nil, fmt.Errorf("tipo de lote inválido")
	}
	
	tamaño := l.Tamaño()
	
	// Confirmar lote
	err := db.ConfirmarLote(lote)
	duracion := time.Since(inicio)
	
	stats := &LoteEstadisticas{
		Entradas:          tamaño,
		DuracionMs:        int64(duracion.Milliseconds()),
		EntradasPorSegundo: float64(tamaño) / duracion.Seconds(),
		Error:             err,
	}
	
	return stats, err
}

// LoteEstadisticas contiene métricas de rendimiento de un lote
type LoteEstadisticas struct {
	Entradas           int
	DuracionMs         int64
	EntradasPorSegundo float64
	Error              error
}