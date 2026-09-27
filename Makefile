.DEFAULT_GOAL := help

APP_ENV ?= dev

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## Run the API (REST :8080, gRPC 127.0.0.1:8082); creates data/ on first boot
	APP_ENV=$(APP_ENV) go run ./cmd/api

.PHONY: build
build: ## Compile bin/api
	@mkdir -p bin
	go build -o bin/api ./cmd/api

.PHONY: test
test: ## Run every test with the race detector
	go test -race ./...

.PHONY: cover
cover: ## Test with a coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

.PHONY: proto
proto: ## Regenerate rpc/ from proto/ (needs buf)
	cd proto && buf lint && buf generate

.PHONY: fmt
fmt: ## gofmt everything except generated code
	gofmt -s -w $$(find . -name '*.go' -not -path './rpc/*')

.PHONY: lint
lint: ## go vet plus a gofmt check
	go vet ./...
	@test -z "$$(gofmt -s -l $$(find . -name '*.go' -not -path './rpc/*'))" || (echo "run make fmt"; exit 1)

.PHONY: clean
clean: ## Delete the local database and uploads
	rm -rf data bin coverage.out
