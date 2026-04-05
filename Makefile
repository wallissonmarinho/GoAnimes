.PHONY: vet check-layout lint test build

vet:
	go vet ./...

check-layout:
	bash scripts/check-layout.sh

# Por defeito usa go run com binário v2.11.4 (compatível com go 1.25 do go.mod). Um golangci-lint no PATH
# compilado só com Go 1.24 falha com "targeted Go version (1.25)" — evita-se assim.
# Para usar o binário global: make lint GOLANGCI_LINT=golangci-lint (precisa ser ≥ v2.11 / Go 1.25).
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4

lint: check-layout vet
	$(GOLANGCI_LINT) run ./...

test:
	go test ./... -count=1

build:
	go build -trimpath -o bin/goanimes ./cmd/goanimes

check: lint test build
