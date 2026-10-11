.PHONY: build install test check check-fixtures check-toolchain demo

check-toolchain:
	python3 scripts/check-toolchain.py

build: check-toolchain
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

install: check-toolchain
	python3 scripts/install-local.py

test: check-toolchain
	go test -race ./...

check: check-toolchain
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	python3 scripts/check-go-fixtures.py
	python3 scripts/check-install.py
	go build -trimpath -o bin/agentspeedbench ./cmd/agentspeedbench

check-fixtures: check-toolchain
	python3 scripts/check-go-fixtures.py

demo: build
	./bin/agentspeedbench run benchmarks/demo.yaml
