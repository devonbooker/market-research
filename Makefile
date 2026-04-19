.PHONY: build test lint tidy clean

build:
	go build -o bin/mr ./cmd/mr

test:
	go test ./... -race

test-short:
	go test ./... -race -short

tidy:
	go mod tidy

lint:
	go vet ./...

clean:
	rm -rf bin/
