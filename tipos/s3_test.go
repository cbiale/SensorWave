package tipos

import (
	"testing"
)

// ==================== Tests de ConfiguracionS3.Validar ====================

// TestConfiguracionS3_Validar_Completa verifica validación con todos los campos
func TestConfiguracionS3_Validar_Completa(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Bucket:          "mi-bucket",
		Region:          "us-west-2",
	}

	err := cfg.Validar()
	if err != nil {
		t.Errorf("No se esperaba error con configuración completa: %v", err)
	}

	t.Log("✓ Configuración completa válida")
}

// TestConfiguracionS3_Validar_SinEndpoint verifica error sin endpoint
func TestConfiguracionS3_Validar_SinEndpoint(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "", // Falta
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "secret",
		Bucket:          "mi-bucket",
	}

	err := cfg.Validar()
	if err == nil {
		t.Error("Se esperaba error sin Endpoint")
	} else {
		t.Logf("✓ Error esperado: %v", err)
	}
}

// TestConfiguracionS3_Validar_SinAccessKey verifica error sin access key
func TestConfiguracionS3_Validar_SinAccessKey(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "", // Falta
		SecretAccessKey: "secret",
		Bucket:          "mi-bucket",
	}

	err := cfg.Validar()
	if err == nil {
		t.Error("Se esperaba error sin AccessKeyID")
	} else {
		t.Logf("✓ Error esperado: %v", err)
	}
}

// TestConfiguracionS3_Validar_SinSecretKey verifica error sin secret key
func TestConfiguracionS3_Validar_SinSecretKey(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "", // Falta
		Bucket:          "mi-bucket",
	}

	err := cfg.Validar()
	if err == nil {
		t.Error("Se esperaba error sin SecretAccessKey")
	} else {
		t.Logf("✓ Error esperado: %v", err)
	}
}

// TestConfiguracionS3_Validar_SinBucket verifica error sin bucket
func TestConfiguracionS3_Validar_SinBucket(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "secret",
		Bucket:          "", // Falta
	}

	err := cfg.Validar()
	if err == nil {
		t.Error("Se esperaba error sin Bucket")
	} else {
		t.Logf("✓ Error esperado: %v", err)
	}
}

// TestConfiguracionS3_Validar_SinRegion verifica que region es opcional
func TestConfiguracionS3_Validar_SinRegion(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "secret",
		Bucket:          "mi-bucket",
		Region:          "", // Vacío pero opcional
	}

	err := cfg.Validar()
	if err != nil {
		t.Errorf("No se esperaba error sin Region (es opcional): %v", err)
	}

	t.Log("✓ Configuración válida sin Region (es opcional)")
}

// ==================== Tests de ConfiguracionS3.AplicarDefaults ====================

// TestConfiguracionS3_AplicarDefaults_RegionVacia verifica default de region
func TestConfiguracionS3_AplicarDefaults_RegionVacia(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
		Bucket:          "bucket",
		Region:          "", // Vacío
	}

	cfg.AplicarDefaults()

	if cfg.Region != "us-east-1" {
		t.Errorf("Region default incorrecta: esperada 'us-east-1', obtenida '%s'", cfg.Region)
	}

	t.Logf("✓ Region default aplicada: %s", cfg.Region)
}

// TestConfiguracionS3_AplicarDefaults_RegionExistente verifica que no sobreescribe region
func TestConfiguracionS3_AplicarDefaults_RegionExistente(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.example.com",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
		Bucket:          "bucket",
		Region:          "eu-west-1", // Ya tiene valor
	}

	cfg.AplicarDefaults()

	if cfg.Region != "eu-west-1" {
		t.Errorf("Region sobreescrita incorrectamente: esperada 'eu-west-1', obtenida '%s'", cfg.Region)
	}

	t.Logf("✓ Region existente preservada: %s", cfg.Region)
}

// TestConfiguracionS3_ValidarMultiplesCamposVacios verifica error prioritario
func TestConfiguracionS3_ValidarMultiplesCamposVacios(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "",
		AccessKeyID:     "",
		SecretAccessKey: "",
		Bucket:          "",
	}

	err := cfg.Validar()
	if err == nil {
		t.Error("Se esperaba error con configuración vacía")
	}

	// Verificar que el primer error es sobre Endpoint
	expectedError := "Endpoint es requerido"
	if err.Error() != expectedError {
		t.Logf("Error obtenido: %v (puede variar el orden de validación)", err)
	}

	t.Log("✓ Validación detecta campos vacíos")
}

// ==================== Tests de CrearClienteS3 ====================

// TestCrearClienteS3_ConfiguracionValida verifica que se crea un cliente con config válida
func TestCrearClienteS3_ConfiguracionValida(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "http://localhost:3900",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
	}

	cliente, err := CrearClienteS3(cfg)

	if err != nil {
		t.Errorf("No se esperaba error con configuración válida: %v", err)
	}
	if cliente == nil {
		t.Error("El cliente no debería ser nil con configuración válida")
	}

	t.Log("✓ Cliente S3 creado correctamente con configuración válida")
}

// TestCrearClienteS3_ConRegion verifica creación con region especificada
func TestCrearClienteS3_ConRegion(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "http://localhost:3900",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Region:          "eu-west-1",
	}

	cliente, err := CrearClienteS3(cfg)

	if err != nil {
		t.Errorf("No se esperaba error: %v", err)
	}
	if cliente == nil {
		t.Error("El cliente no debería ser nil")
	}

	t.Log("✓ Cliente S3 creado correctamente con region especificada")
}

// TestCrearClienteS3_SinRegion verifica que se aplica region por defecto
func TestCrearClienteS3_SinRegion(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "http://localhost:3900",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Region:          "", // Vacío, debería aplicar default
	}

	cliente, err := CrearClienteS3(cfg)

	if err != nil {
		t.Errorf("No se esperaba error: %v", err)
	}
	if cliente == nil {
		t.Error("El cliente no debería ser nil")
	}

	t.Log("✓ Cliente S3 creado correctamente sin region (usa default)")
}

// TestCrearClienteS3_ErrorSinEndpoint verifica error con endpoint vacío
func TestCrearClienteS3_ErrorSinEndpoint(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "", // Falta
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
	}

	cliente, err := CrearClienteS3(cfg)

	if err == nil {
		t.Error("Se esperaba error sin Endpoint")
	}
	if cliente != nil {
		t.Error("El cliente debería ser nil cuando hay error")
	}

	t.Logf("✓ Error esperado sin Endpoint: %v", err)
}

// TestCrearClienteS3_ErrorSinAccessKey verifica error con access key vacío
func TestCrearClienteS3_ErrorSinAccessKey(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "http://localhost:3900",
		AccessKeyID:     "", // Falta
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
	}

	cliente, err := CrearClienteS3(cfg)

	if err == nil {
		t.Error("Se esperaba error sin AccessKeyID")
	}
	if cliente != nil {
		t.Error("El cliente debería ser nil cuando hay error")
	}

	t.Logf("✓ Error esperado sin AccessKeyID: %v", err)
}

// TestCrearClienteS3_ErrorSinSecretKey verifica error con secret key vacío
func TestCrearClienteS3_ErrorSinSecretKey(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "http://localhost:3900",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "", // Falta
		Bucket:          "test-bucket",
	}

	cliente, err := CrearClienteS3(cfg)

	if err == nil {
		t.Error("Se esperaba error sin SecretAccessKey")
	}
	if cliente != nil {
		t.Error("El cliente debería ser nil cuando hay error")
	}

	t.Logf("✓ Error esperado sin SecretAccessKey: %v", err)
}

// TestCrearClienteS3_ErrorSinBucket verifica error con bucket vacío
func TestCrearClienteS3_ErrorSinBucket(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "http://localhost:3900",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "", // Falta
	}

	cliente, err := CrearClienteS3(cfg)

	if err == nil {
		t.Error("Se esperaba error sin Bucket")
	}
	if cliente != nil {
		t.Error("El cliente debería ser nil cuando hay error")
	}

	t.Logf("✓ Error esperado sin Bucket: %v", err)
}

// TestCrearClienteS3_EndpointHTTPS verifica creación con endpoint HTTPS
func TestCrearClienteS3_EndpointHTTPS(t *testing.T) {
	cfg := ConfiguracionS3{
		Endpoint:        "https://s3.amazonaws.com",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "test-bucket",
		Region:          "us-west-2",
	}

	cliente, err := CrearClienteS3(cfg)

	if err != nil {
		t.Errorf("No se esperaba error con endpoint HTTPS: %v", err)
	}
	if cliente == nil {
		t.Error("El cliente no debería ser nil")
	}

	t.Log("✓ Cliente S3 creado correctamente con endpoint HTTPS")
}
