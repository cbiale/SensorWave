package sensorwave

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	_ "github.com/mattn/go-sqlite3"
)

// DB es la estructura que contiene las conexiones y configuraciones necesarias
type DB struct {
	SQLiteDB       *sql.DB
	MinioClient    *minio.Client
	Compression    string
	SegmentLength  int64
	RetentionTime  int64
}

// createTables crea las tablas necesarias si no existen
func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS node_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id INTEGER NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY(node_id) REFERENCES nodes(id)
		)`,
		`CREATE TABLE IF NOT EXISTS sensors (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			metric_type TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sensors_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sensor_id INTEGER NOT NULL,
			node_id INTEGER NOT NULL,
			value BLOB NOT NULL,
			timestamp DATETIME NOT NULL,
			FOREIGN KEY(sensor_id) REFERENCES sensors(id),
			FOREIGN KEY(node_id) REFERENCES nodes(id)
		)`,
		`CREATE TABLE IF NOT EXISTS actuators (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS actuator_states_definitions (
    		id INTEGER PRIMARY KEY AUTOINCREMENT,
    		actuator_id INTEGER NOT NULL,
    		state TEXT NOT NULL,
	 		FOREIGN KEY(actuator_id) REFERENCES actuators(id)
		)`,
		`CREATE TABLE IF NOT EXISTS actuator_states (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actuator_id INTEGER NOT NULL,
			node_id INTEGER NOT NULL,
			state TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			FOREIGN KEY(actuator_id) REFERENCES actuators(id),
			FOREIGN KEY(node_id) REFERENCES nodes(id)
		)`,
	} 

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			return fmt.Errorf("error al crear la tabla: %w", err)
		}
	}
	return nil
}

// Init inicializa una conexión a la base de datos SQLite y configura los parámetros para el almacenamiento en la nube
func Init(dbFileName, s3URL, accessKey, secretKey, compression string, segmentLength, retentionTime int64) (*DB, error) {
	// Validar el tipo de compresión
	validCompressions := map[string]bool{
		"gzip":   true,
		"snappy": true,
		"lz4":    true,
		"none":   true,
	}
	if !validCompressions[compression] {
		return nil, fmt.Errorf("tipo de compresión no válido: %s", compression)
	}

	// Validar que el largo del segmento y el tiempo de retención no sean menores a cero
	if segmentLength < 0 {
		return nil, fmt.Errorf("el largo del segmento no puede ser menor a cero: %d", segmentLength)
	}
	if retentionTime < 0 {
		return nil, fmt.Errorf("el tiempo de retención no puede ser menor a cero: %d", retentionTime)
	}

	// Conectar a la base de datos SQLite
	sqliteDB, err := sql.Open("sqlite3", dbFileName)
	if err != nil {
		return nil, fmt.Errorf("error al abrir la base de datos SQLite: %w", err)
	}

	// Verificar la conexión
	err = sqliteDB.Ping()
	if err != nil {
		return nil, fmt.Errorf("error al verificar la conexión a SQLite: %w", err)
	}

	// crear tablas necesarias si no existen
	err = createTables(sqliteDB)
	if err != nil {
		return nil, fmt.Errorf("error al crear las tablas necesarias: %w", err)
	}

	// Crear el cliente MinIO
	minioClient, err := minio.New(s3URL, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false, // Cambiar a true si se usa HTTPS
	})
	if err != nil {
		return nil, fmt.Errorf("error al crear el cliente MinIO: %w", err)
	}

	// Nombre del bucket predeterminado
	bucketName := "sensorwave"

	// Verificar la existencia del bucket
	exists, err := minioClient.BucketExists(context.Background(), bucketName)
	if err != nil {
		return nil, fmt.Errorf("error al verificar la existencia del bucket: %w", err)
	}

	if !exists {
		// Intentar crear el bucket
		err = minioClient.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("error al crear el bucket '%s': %w", bucketName, err)
		}
	}

	// Crear la estructura DB
	db := &DB{
		SQLiteDB:      sqliteDB,
		MinioClient:   minioClient,
		Compression:   compression,
		SegmentLength: segmentLength,
		RetentionTime: retentionTime,
	}

	return db, nil
}


// Close cierra la conexión a la base de datos SQLite, a Minio y libera los recursos
func (db *DB) Close() error {
	// Cerrar la conexión a la base de datos SQLite
	err := db.SQLiteDB.Close()
	if err != nil {
		return fmt.Errorf("error al cerrar la base de datos SQLite: %w", err)
	}

	// liberar los recursos
	db = nil

	return nil
}
