.PHONY: build run dev

build:
	@go build -o bin/api ./cmd/api
run: build
	@./bin/api
dev:
	@go run ./cmd/api