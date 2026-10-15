.PHONY: help dev run tidy test vet check openapi

help:
	@echo "make dev      - docker compose dev server with hot reload (docs at /docs)"
	@echo "make run      - run locally (needs Go; set JWT_SECRET)"
	@echo "make tidy     - resolve dependencies (creates go.sum)"
	@echo "make check    - vet + tests (run before every commit)"
	@echo "make openapi  - regenerate openapi.json (commit it)"

dev:
	docker compose up --build

run:
	go run ./cmd/server

tidy:
	go mod tidy

vet:
	go vet ./...

test:
	go test -race ./...

check: vet test

openapi:
	go run ./cmd/openapi > openapi.json
