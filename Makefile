# Thin wrappers over the commands the README documents, so the run path is one
# word long and stays identical for a human and for CI.

.PHONY: up down logs rebuild install test test-backend test-frontend lint fmt

up:            ## build and start all three services
	docker compose up --build

down:          ## stop the services, keep the database volume
	docker compose down

logs:          ## follow logs from every service
	docker compose logs -f

rebuild:       ## rebuild images without cache
	docker compose build --no-cache

install:       ## install local toolchains (not needed for docker compose)
	cd frontend && npm ci
	cd backend && go mod download

test: test-backend test-frontend

# Integration tests need a reachable PostgreSQL and create their own throwaway
# databases; without TEST_DATABASE_URL they skip rather than fail.
test-backend:
	cd backend && go test ./...

test-frontend:
	cd frontend && npm run test

lint:
	cd backend && go vet ./...
	cd frontend && npx tsc --noEmit

fmt:
	cd backend && gofmt -l -w .
