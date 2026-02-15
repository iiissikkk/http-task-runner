.PHONY: run up down test docker-build docker-compose-up docker-compose-down migrate-up migrate-down ps

COMPOSE := docker compose

run: up

up:
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down --remove-orphans

test:
	$(COMPOSE) run --rm app go test ./...

docker-build:
	docker build -t http-task-runner:local .

docker-compose-up: up

docker-compose-down: down

migrate-up:
	$(COMPOSE) up -d db
	$(COMPOSE) run --rm app go run ./cmd/migrate up

migrate-down:
	$(COMPOSE) up -d db
	$(COMPOSE) run --rm app go run ./cmd/migrate down

ps:
	$(COMPOSE) ps
