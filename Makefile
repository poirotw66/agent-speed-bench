.PHONY: build install test check check-fixtures demo

build:
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

install:
	python3 scripts/install-local.py

test:
	go test -race ./...

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	python3 scripts/check-go-fixtures.py
	python3 scripts/check-install.py
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

check-fixtures:
	python3 scripts/check-go-fixtures.py

demo: build
	./bin/agentspeedbench run benchmarks/demo.yaml
