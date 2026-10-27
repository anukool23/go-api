.PHONY: build run dev clean

build:
	@go build -o bin/api ./cmd/api
run: build
	@./bin/api
dev:
	@go run ./cmd/api
vet:
	@go vet ./...
migrate-up:
	go run ./cmd/migrate up
migrate-down:
	go run ./cmd/migrate down