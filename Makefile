.PHONY: build run dev

build:
	@go build -o bin/api ./cmd/api
run: build
	@./bin/api
dev:
	@go run ./cmd/api
migrate-up:
	go run ./cmd/migrate up
migrate-down:
	go run ./cmd/migrate down