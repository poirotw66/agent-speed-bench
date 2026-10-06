.PHONY: build test check demo

build:
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

test:
	go test -race ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

demo: build
	./bin/agentspeedbench run benchmarks/demo.yaml
