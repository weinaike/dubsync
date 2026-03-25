# Timeline Test Execution and Reporting System
# ============================================
# Usage: make [target]

# Project variables
PROJECT_NAME := timeline
GO := go
GOFLAGS := -v
PKG := ./...
SRC_DIR := src

# Output directories
REPORT_DIR := test-reports
ARCHIVE_DIR := $(REPORT_DIR)/archive/$(shell date +%Y-%m-%d)
GOLDEN_DIR := $(REPORT_DIR)/golden

# Go test flags
TEST_FLAGS := -short
VERBOSE_FLAGS := -v
COVERAGE_FLAGS := -coverprofile=$(REPORT_DIR)/coverage.out -covermode=atomic
RACE_FLAGS := -race
INTEGRATION_FLAGS := -run "Integration|E2E"

# Coverage thresholds
COVERAGE_MIN := 70

# Timeout settings
TEST_TIMEOUT := 5m
INTEGRATION_TIMEOUT := 10m

# ==========================================
# Default target
# ==========================================
.PHONY: help
help:
	@echo "Timeline Test Execution System"
	@echo "==============================="
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Test Execution:"
	@echo "  test              Run all unit tests (fast)"
	@echo "  test-verbose      Run tests with verbose output"
	@echo "  test-coverage     Run tests with coverage report"
	@echo "  test-race         Run tests with race detection"
	@echo "  test-integration  Run integration tests (slower)"
	@echo "  test-all          Run all tests including integration"
	@echo ""
	@echo "Reporting:"
	@echo "  report            Generate test reports (JSON, JUnit, HTML)"
	@echo "  report-archive    Archive reports with timestamp"
	@echo "  coverage-html     Generate HTML coverage report"
	@echo "  coverage-check    Verify coverage meets minimum threshold"
	@echo ""
	@echo "Benchmarks:"
	@echo "  benchmark         Run all benchmarks"
	@echo "  benchmark-solver  Run solver-specific benchmarks"
	@echo ""
	@echo "CI/CD:"
	@echo "  ci                Full CI pipeline (lint + test + coverage + report)"
	@echo "  golden-update     Update golden files with current results"
	@echo "  golden-verify     Verify results against golden files"
	@echo ""
	@echo "Utility:"
	@echo "  clean             Clean generated files"
	@echo "  fmt               Format code"
	@echo "  lint              Run golangci-lint"
	@echo "  tidy              Run go mod tidy"

# ==========================================
# Test Execution Targets
# ==========================================

.PHONY: test
test:
	@echo ">>> Running unit tests..."
	@$(GO) test $(PKG) $(TEST_FLAGS)

.PHONY: test-verbose
test-verbose:
	@echo ">>> Running tests (verbose)..."
	@$(GO) test $(PKG) $(VERBOSE_FLAGS)

.PHONY: test-coverage
test-coverage:
	@echo ">>> Running tests with coverage..."
	@mkdir -p $(REPORT_DIR)
	@$(GO) test $(PKG) $(COVERAGE_FLAGS) $(VERBOSE_FLAGS)
	@$(GO) tool cover -func=$(REPORT_DIR)/coverage.out | tee $(REPORT_DIR)/coverage.txt
	@echo ""
	@echo "Coverage Summary:"
	@cat $(REPORT_DIR)/coverage.txt

.PHONY: test-race
test-race:
	@echo ">>> Running tests with race detection..."
	@$(GO) test $(PKG) $(RACE_FLAGS) $(VERBOSE_FLAGS)

.PHONY: test-integration
test-integration:
	@echo ">>> Running integration tests..."
	@$(GO) test $(PKG) $(INTEGRATION_FLAGS) -timeout $(INTEGRATION_TIMEOUT) $(VERBOSE_FLAGS)

.PHONY: test-all
test-all: test-coverage test-integration
	@echo ""
	@echo ">>> All tests completed."

# ==========================================
# Reporting Targets
# ==========================================

.PHONY: report
report:
	@echo ">>> Generating test reports..."
	@mkdir -p $(REPORT_DIR)
	@chmod +x run_tests.sh
	./run_tests.sh -f all -c -o $(REPORT_DIR)
	@echo ""
	@echo "Reports generated in $(REPORT_DIR)/"
	@ls -la $(REPORT_DIR)/

.PHONY: report-archive
report-archive:
	@echo ">>> Archiving test reports..."
	@mkdir -p $(ARCHIVE_DIR)
	@chmod +x run_tests.sh
	./run_tests.sh -f all -c -o $(ARCHIVE_DIR)
	@echo ""
	@echo "Reports archived to $(ARCHIVE_DIR)/"

.PHONY: coverage-html
coverage-html: test-coverage
	@echo ">>> Generating HTML coverage report..."
	@$(GO) tool cover -html=$(REPORT_DIR)/coverage.out -o $(REPORT_DIR)/coverage.html
	@echo "HTML coverage report: $(REPORT_DIR)/coverage.html"

.PHONY: coverage-check
coverage-check: test-coverage
	@echo ">>> Checking coverage threshold..."
	@COVERAGE=$$($(GO) tool cover -func=$(REPORT_DIR)/coverage.out | grep total | awk '{print $$3}' | tr -d '%'); \
	echo "Total coverage: $${COVERAGE}%"; \
	if [ "$$(printf '%s\n' "$${COVERAGE} < $(COVERAGE_MIN)" | bc -l 2>/dev/null || echo 0)" -eq 1 ]; then \
		echo "ERROR: Coverage $${COVERAGE}% is below minimum $(COVERAGE_MIN)%"; \
		exit 1; \
	fi; \
	echo "OK: Coverage $${COVERAGE}% meets minimum $(COVERAGE_MIN)%"

# ==========================================
# Benchmark Targets
# ==========================================

.PHONY: benchmark
benchmark:
	@echo ">>> Running benchmarks..."
	@mkdir -p $(REPORT_DIR)
	@$(GO) test $(PKG) -bench=. -benchmem -benchtime=2s | tee $(REPORT_DIR)/benchmark.txt
	@echo ""
	@echo "Benchmark results saved to $(REPORT_DIR)/benchmark.txt"

.PHONY: benchmark-solver
benchmark-solver:
	@echo ">>> Running solver benchmarks..."
	@$(GO) test ./src/solver/... -bench=. -benchmem -benchtime=3s

# ==========================================
# CI/CD Targets
# ==========================================

.PHONY: ci
ci: clean fmt lint test-coverage coverage-check report
	@echo ""
	@echo ">>> CI pipeline completed successfully."

.PHONY: golden-update
golden-update:
	@echo ">>> Updating golden files..."
	@mkdir -p $(GOLDEN_DIR)
	@chmod +x run_tests.sh
	./run_tests.sh -f json -o $(GOLDEN_DIR) --update-golden
	@echo "Golden files updated in $(GOLDEN_DIR)/"

.PHONY: golden-verify
golden-verify:
	@echo ">>> Verifying against golden files..."
	@chmod +x run_tests.sh
	./run_tests.sh -f json --verify --golden-dir $(GOLDEN_DIR)

# ==========================================
# Utility Targets
# ==========================================

.PHONY: clean
clean:
	@echo ">>> Cleaning generated files..."
	@rm -rf $(REPORT_DIR)
	@echo "Clean complete."

.PHONY: fmt
fmt:
	@echo ">>> Formatting code..."
	@$(GO) fmt $(PKG)
	@echo "Format complete."

.PHONY: lint
lint:
	@echo ">>> Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(PKG); \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

.PHONY: tidy
tidy:
	@echo ">>> Running go mod tidy..."
	@$(GO) mod tidy
	@echo "Tidy complete."

.PHONY: check
check: fmt lint test
	@echo ""
	@echo ">>> All checks passed."
