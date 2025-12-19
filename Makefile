# Makefile para SensorWave - Sistema de Almacenamiento IoT

.PHONY: help test test-unit test-integration test-coverage test-run clean info graficos

GO := go
PYTHON := python3
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
TEST_TIMEOUT := 30m
COVERAGE_PACKAGES := ./compresor/...,./tipos/...,./edge/...,./despachador/...,./middleware/servidor/...

.DEFAULT_GOAL := help

help:
	@echo "SensorWave - Comandos Disponibles"
	@echo ""
	@echo "Testing:"
	@echo "  make test             - Ejecuta todos los tests"
	@echo "  make test-unit        - Solo tests unitarios"
	@echo "  make test-integration - Solo tests de integración"
	@echo "  make test-coverage    - Tests + reporte de cobertura HTML"
	@echo "  make test-run NAME=X  - Ejecuta tests que coincidan con X"
	@echo ""
	@echo "Otros:"
	@echo "  make graficos         - Genera gráficos para la tesis"
	@echo "  make clean            - Limpia artefactos"
	@echo "  make info             - Información del proyecto"

test:
	$(GO) test -v -timeout $(TEST_TIMEOUT) ./test/unit/... ./test/integration/...

test-unit:
	$(GO) test -v -timeout $(TEST_TIMEOUT) ./test/unit/...

test-integration:
	$(GO) test -v -timeout $(TEST_TIMEOUT) ./test/integration/...

test-run:
	$(GO) test -v -timeout $(TEST_TIMEOUT) -run $(NAME) ./test/unit/... ./test/integration/...

test-coverage:
	-$(GO) test -timeout $(TEST_TIMEOUT) -coverprofile=$(COVERAGE_FILE) -coverpkg=$(COVERAGE_PACKAGES) ./test/unit/... ./test/integration/...
	@$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@$(GO) tool cover -func=$(COVERAGE_FILE) | grep total
	@echo "Reporte generado: $(COVERAGE_HTML)"

graficos:
	@echo "Generando gráficos para la tesis..."
	$(PYTHON) test/integration/edge/series/generar_graficos.py
	@echo "Gráficos generados"

clean:
	@find . -name "*.db" -path "./test/*" -type d -exec rm -rf {} + 2>/dev/null || true
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML) 2>/dev/null || true
	@echo "Limpieza completada"

info:
	@echo "SensorWave - Go: $(shell $(GO) version | awk '{print $$3}')"
	@echo "Tests: $(shell find ./test -name '*_test.go' 2>/dev/null | wc -l) archivos"
