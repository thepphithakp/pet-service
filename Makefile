SHELL := /bin/bash
APP    := pet-service
PKG    := github.com/vertex/pet-service
COMPOSE := docker compose -f docker-compose.dev.yml

.DEFAULT_GOAL := help
.PHONY: help build run test test-integration cover lint vet tidy fmt \
        db-up db-down db-reset migrate migrate-info migrate-validate \
        docker-dev clean

help: ## แสดงคำสั่งทั้งหมด
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## build binary
	CGO_ENABLED=0 go build -o bin/$(APP) ./cmd/server

run: ## รัน service (ต้องมี .env)
	go run ./cmd/server

fmt: ## gofmt
	gofmt -w -s $$(find . -name '*.go' -not -path './vendor/*')

vet: ## go vet
	go vet ./...

# ตรึงเวอร์ชันให้ตรงกับที่ CI ใช้ ไม่งั้น lint ผ่านในเครื่องแต่แดงบน CI
GOLANGCI_VERSION ?= v2.13.1

lint: ## golangci-lint (ใช้ binary ในเครื่องถ้ามี ไม่มีก็ใช้ docker)
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "ไม่พบ golangci-lint ในเครื่อง — ใช้ docker $(GOLANGCI_VERSION) แทน"; \
		docker run --rm -v "$(PWD)":/app -w /app \
			golangci/golangci-lint:$(GOLANGCI_VERSION) golangci-lint run ./...; \
	fi

tidy: ## go mod tidy + ตรวจว่าไม่มี diff
	go mod tidy
	@git diff --exit-code go.mod go.sum || { echo "go.mod/go.sum ไม่ tidy — commit การเปลี่ยนแปลงด้วย"; exit 1; }

test: ## unit test (ไม่ต้องใช้ docker)
	go test -race -count=1 ./...

test-integration: ## integration test (ต้องมี postgres — ใช้ make db-up ก่อน)
	TEST_DATABASE_URL="$${TEST_DATABASE_URL:-postgres://vertex:vertex@localhost:55432/vertex?sslmode=disable&search_path=pet}" \
	go test -race -count=1 -tags=integration ./...

cover: ## test + coverage report
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1

db-up: ## ยก postgres + รัน flyway migration (ต้อง clone vertex-migrations ไว้ข้างกัน)
	@test -d ../vertex-migrations || { echo "ไม่พบ ../vertex-migrations — clone repo migration ไว้ข้างกันก่อน"; exit 1; }
	$(COMPOSE) up -d postgres
	$(COMPOSE) run --rm flyway

db-down: ## ปิด + ลบ volume
	$(COMPOSE) down -v

db-reset: db-down db-up ## ล้างแล้วสร้างใหม่

migrate: ## รัน flyway migrate
	$(COMPOSE) run --rm flyway migrate

migrate-info: ## flyway info
	$(COMPOSE) run --rm flyway info

migrate-validate: ## flyway validate
	$(COMPOSE) run --rm flyway validate

docker-dev: ## ยกทั้ง stack (postgres + flyway + app)
	$(COMPOSE) up --build

clean: ## ลบ artifact
	rm -rf bin coverage.out coverage.html
