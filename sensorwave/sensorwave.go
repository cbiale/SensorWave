// Package sensorwave implementa la lógica para manejar sensores y actuadores,
// incluyendo almacenamiento local y en la nube.
package sensorwave

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/cbiale/sensorwave/sensorwave/models"
	_ "github.com/mattn/go-sqlite3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// DB contiene la conexión a la base de datos y configuraciones de almacenamiento.
type DB struct {
	SQLiteDB      *sql.DB
	MinioClient   *minio.Client
	Compression   string
	SegmentLength int64
	RetentionTime int64
}

// validCompressions define los tipos válidos de compresión admitidos por la biblioteca
var validCompressions = map[string]bool{
	"gzip":   true,
	"snappy": true,
	"lz4":    true,
	"none":   true,
}

// createTables crea las tablas necesarias en el nodo al borde si no existen
func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
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
			name TEXT NOT NULL UNIQUE,
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
			name TEXT NOT NULL UNIQUE
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

// SENSORES

// RegisterSensor registra un tipo de sensor
func (db *DB) RegisterSensor(name, metricType string) (int64, error) {
	result, err := db.SQLiteDB.Exec("INSERT INTO sensors (name, metric_type) VALUES (?, ?)", name, metricType)
	if err != nil {
		return 0, fmt.Errorf("error al insertar un tipo de sensor: %w", err)
	}
	return result.LastInsertId()
}

// GetSensors obtiene los tipos de sensores registrados
func (db *DB) GetSensors() ([]models.Sensor, error) {
	rows, err := db.SQLiteDB.Query("SELECT * FROM sensors ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("error al obtener tipos de sensores: %w", err)
	}
	defer rows.Close()

	// para almacenar los resultados
	var sensors []models.Sensor

	for rows.Next() {
		var sensor models.Sensor
		if err := rows.Scan(&sensor.Id, &sensor.Name, &sensor.MetricType); err != nil {
			return nil, fmt.Errorf("error al recuperar una fila: %w", err)
		}
		sensors = append(sensors, sensor)
	}
	return sensors, nil
}


// InsertSensorData inserta un nuevo valor de sensor en la base de datos.
// Valida que el nodo y el sensor existan antes de la inserción.
// Si el número de registros en sensors_data alcanza el límite definido en segmentLength, se activará la compresión.
func (db *DB) InsertSensorData(nodeID int, sensorID int, timestamp *time.Time, value []byte) error {
	// Verificar que el nodo existe
	var nodeCount int
	err := db.SQLiteDB.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", nodeID).Scan(&nodeCount)
	if err != nil {
		return fmt.Errorf("error al verificar la existencia del nodo: %w", err)
	}
	if nodeCount == 0 {
		return fmt.Errorf("el nodo con ID %d no existe", nodeID)
	}

	// Verificar que el sensor existe
	var sensorCount int
	err = db.SQLiteDB.QueryRow("SELECT COUNT(*) FROM sensors WHERE id = ?", sensorID).Scan(&sensorCount)
	if err != nil {
		return fmt.Errorf("error al verificar la existencia del sensor: %w", err)
	}
	if sensorCount == 0 {
		return fmt.Errorf("el sensor con ID %d no existe", sensorID)
	}

	// Usar la hora actual si no se proporciona una
	if timestamp == nil {
		now := time.Now()
		timestamp = &now
	}

	// Iniciar transacción
	tx, err := db.SQLiteDB.Begin()
	if err != nil {
		return fmt.Errorf("error al iniciar la transacción: %w", err)
	}

	// Insertar el valor del sensor
	_, err = tx.Exec("INSERT INTO sensors_data (sensor_id, node_id, value, timestamp) VALUES (?, ?, ?, ?)",
		sensorID, nodeID, value, timestamp)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error al insertar datos del sensor: %w", err)
	}

	// Contar el número de registros en sensors_data
	var recordCount int64
	err = tx.QueryRow("SELECT COUNT(*) FROM sensors_data WHERE sensor_id = ?", sensorID).Scan(&recordCount)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("error al contar registros en sensors_data: %w", err)
	}

	// Confirmar la transacción antes de activar la compresión
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error al confirmar la transacción: %w", err)
	}

	// Si se alcanzó el límite de segmentLength, activar la compresión (a definir después)
	if recordCount >= db.SegmentLength {
//		go db.compressSensorData(sensorID) // Llamada asíncrona al compresor (implementaremos después)
	}

	return nil
}

// ACTUADORES

// RegisterActuator registra un actuador
func (db *DB) RegisterActuator(name string, states []string) (int64, error) {

	// Iniciar la transacción
	tx, err := db.SQLiteDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("error starting transaction: %w", err)
	}

	// Insertar el actuador
	result, err := tx.Exec("INSERT INTO actuators (name) VALUES (?)", name)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("error al insertar un tipo de actuator: %w", err)
	}
	actuatorID, _ := result.LastInsertId()

	// Insertar los estados posibles
	for _, state := range states {
		_, err := tx.Exec("INSERT INTO actuator_states_definitions (actuator_id, state) VALUES (?, ?)", actuatorID, state)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("error al insertar un estado de actuador: %w", err)
		}
	}

	// Commit de la transacción
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("error committing transaction: %w", err)
	}

	return actuatorID, nil
}

// InsertActuatorState inserta un cambio de actuador
func (db *DB) InsertActuatorState(actuatorID int, state string) error {
	// Verificar que el estado esté definido para el actuador
	var count int
	err := db.SQLiteDB.QueryRow("SELECT COUNT(*) FROM actuator_states_definitions WHERE actuator_id = ? AND state = ?", actuatorID, state).Scan(&count)
	if err != nil {
		return fmt.Errorf("error al controlar el tipo de estado: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("el estado '%s' no se encuentra definido para el actuator con ID %d", state, actuatorID)
	}

	// Insertar el cambio de estado
	_, err = db.SQLiteDB.Exec("INSERT INTO actuator_states (actuator_id, state, timestamp) VALUES (?, ?, ?)", actuatorID, state, time.Now())
	if err != nil {
		return fmt.Errorf("error al insertar un estado de un actuador: %w", err)
	}
	return nil
}

// GetActuators obtiene los actuadores registrados junto con sus estados definidos
func (db *DB) GetActuators() ([]models.Actuator, error) {
	// Consulta para obtener los actuadores
	rows, err := db.SQLiteDB.Query("SELECT id, name FROM actuators ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("error al obtener actuadores: %w", err)
	}
	defer rows.Close()

	var actuators []models.Actuator

	// Recorrer los actuadores
	for rows.Next() {
		var actuator models.Actuator

		if err := rows.Scan(&actuator.Id, &actuator.Name); err != nil {
			return nil, fmt.Errorf("error al recuperar una fila de actuadores: %w", err)
		}

		// Obtener los estados para cada actuador
		stateRows, err := db.SQLiteDB.Query("SELECT state FROM actuator_states_definitions WHERE actuator_id = ?", actuator.Id)
		if err != nil {
			return nil, fmt.Errorf("error al obtener estados del actuador con ID %d: %w", actuator.Id, err)
		}

		var states []string
		for stateRows.Next() {
			var state string
			if err := stateRows.Scan(&state); err != nil {
				stateRows.Close()
				return nil, fmt.Errorf("error al recuperar un estado: %w", err)
			}
			states = append(states, state)
		}
		stateRows.Close()

		// Asignar los estados al actuador
		actuator.States = states
		actuators = append(actuators, actuator)
	}

	return actuators, nil
}

// RegisterNode registra un nodo.
func (db *DB) RegisterNode(name string, metadata map[string]string) (int64, error) {
	// Iniciar una transacción
	tx, err := db.SQLiteDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("error al iniciar la transacción: %w", err)
	}

	// Insertar el nodo en la tabla nodes
	result, err := tx.Exec("INSERT INTO nodes (name) VALUES (?)", name)
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("error al insertar el nodo: %w", err)
	}
	nodeID, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		return 0, fmt.Errorf("error al obtener el ID del nodo: %w", err)
	}

	// Insertar los metadatos en la tabla node_metadata
	for key, value := range metadata {
		_, err := tx.Exec("INSERT INTO node_metadata (node_id, key, value) VALUES (?, ?, ?)", nodeID, key, value)
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("error al insertar metadatos para el nodo %d: %w", nodeID, err)
		}
	}

	// Confirmar la transacción
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("error al confirmar la transacción: %w", err)
	}

	return nodeID, nil
}

// GetNodes obtiene los nodos registrados junto con sus metadatos
func (db *DB) GetNodes() ([]models.Node, error) {
	// Consulta para obtener los nodos
	rows, err := db.SQLiteDB.Query("SELECT id, name FROM nodes ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("error al obtener nodos: %w", err)
	}
	defer rows.Close()

	var nodes []models.Node

	// Recorrer los nodos
	for rows.Next() {
		var node models.Node
		if err := rows.Scan(&node.Id, &node.Name); err != nil {
			return nil, fmt.Errorf("error al recuperar una fila de nodos: %w", err)
		}

		// Obtener los metadatos para cada nodo
		metadataRows, err := db.SQLiteDB.Query("SELECT key, value FROM node_metadata WHERE node_id = ?", node.Id)
		if err != nil {
			return nil, fmt.Errorf("error al obtener metadatos del nodo con ID %d: %w", node.Id, err)
		}

		metadata := make(map[string]string)
		for metadataRows.Next() {
			var key, value string
			if err := metadataRows.Scan(&key, &value); err != nil {
				metadataRows.Close()
				return nil, fmt.Errorf("error al recuperar un metadato: %w", err)
			}
			metadata[key] = value
		}
		metadataRows.Close()

		// Asignar los metadatos al nodo
		node.Metadata = metadata
		nodes = append(nodes, node)
	}

	return nodes, nil
}
