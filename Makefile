.PHONY: build test check check-fixtures demo

build:
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

test:
	go test -race ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	python3 scripts/check-go-fixtures.py
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

check-fixtures:
	python3 scripts/check-go-fixtures.py

demo: build
	./bin/agentspeedbench run benchmarks/demo.yaml
