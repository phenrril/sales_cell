APP=tienda3d
PKG=github.com/phenrril/tienda3d
PORT?=8080

.PHONY: dev build test run docker-build docker-run db-up tidy

dev:
	go run ./cmd/tienda3d

build:
	go build -o bin/$(APP) ./cmd/tienda3d

test:
	go test ./... -count=1 -timeout=60s

run: build
	PORT=$(PORT) ./bin/$(APP)

docker-build:
	docker build -t $(APP):latest .

docker-run:
	docker compose up --build

lint:
	go vet ./...

tidy:
	go mod tidy

print-env:
	@echo "Using PORT=$(PORT)"

