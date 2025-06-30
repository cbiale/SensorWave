package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"edgesensorwave/internal/admin"
	edgesensorwave "edgesensorwave/pkg"
)

func main() {
	// Configuración de flags
	var (
		dbPath   = flag.String("db-path", "./sensores.esw", "Ruta a la base de datos EdgeSensorWave")
		port     = flag.String("port", "8080", "Puerto del servidor web")
		host     = flag.String("host", "localhost", "Host del servidor web")
		devMode  = flag.Bool("dev", false, "Modo desarrollo (recarga automática)")
	)
	flag.Parse()

	log.Printf("🚀 Iniciando EdgeSensorWave Admin...")
	log.Printf("📊 Base de datos: %s", *dbPath)
	log.Printf("🌐 Servidor: http://%s:%s", *host, *port)

	// Abrir base de datos EdgeSensorWave
	db, err := edgesensorwave.Abrir(*dbPath, edgesensorwave.OpcionesDefecto())
	if err != nil {
		log.Fatalf("❌ Error abriendo base de datos: %v", err)
	}
	defer func() {
		log.Println("🔒 Cerrando base de datos...")
		db.Cerrar()
	}()

	// Crear servidor admin
	adminServer := admin.NewServer(db, &admin.Config{
		Host:    *host,
		Port:    *port,
		DevMode: *devMode,
	})

	// Configurar rutas
	mux := http.NewServeMux()
	adminServer.RegisterRoutes(mux)

	// Servidor HTTP
	server := &http.Server{
		Addr:         *host + ":" + *port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Canal para manejar señales de sistema
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar servidor en goroutine
	go func() {
		log.Printf("✅ Servidor iniciado en http://%s:%s", *host, *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Error del servidor: %v", err)
		}
	}()

	// Esperar señal de cierre
	<-quit
	log.Println("🛑 Cerrando servidor...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("❌ Error cerrando servidor: %v", err)
	}

	log.Println("✅ Servidor cerrado correctamente")
}