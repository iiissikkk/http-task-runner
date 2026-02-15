.PHONY: run test docker-build docker-compose-up docker-compose-down migrate-up migrate-down

run:
	go run ./cmd/server

test:
	go test ./...

docker-build:
	docker build -t http-task-runner:local .

docker-compose-up:
	docker compose up --build -d

docker-compose-down:
	docker compose down

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down
