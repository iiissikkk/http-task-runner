.PHONY: run up down test test-unit test-integration test-integration-store test-integration-http test-all docker-build docker-compose-up docker-compose-down ps oapi-gen

COMPOSE := docker compose

run: up

up:
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down --remove-orphans

test: test-unit

test-unit:
	$(COMPOSE) run --rm app go test ./...

test-integration:
	$(COMPOSE) run --rm --no-deps \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-e DOCKER_HOST=unix:///var/run/docker.sock \
		app go test -tags=integration ./internal/repository/postgres ./internal/delivery/http -v

test-integration-store:
	$(COMPOSE) run --rm --no-deps \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-e DOCKER_HOST=unix:///var/run/docker.sock \
		app go test -tags=integration ./internal/repository/postgres -v

test-integration-http:
	$(COMPOSE) run --rm --no-deps \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-e DOCKER_HOST=unix:///var/run/docker.sock \
		app go test -tags=integration ./internal/delivery/http -v

test-all: test-unit test-integration

docker-build:
	docker build -t http-task-runner:local .

docker-compose-up: up

docker-compose-down: down

ps:
	$(COMPOSE) ps

oapi-gen:
	$(COMPOSE) run --rm --no-deps \
		--user "$$(id -u):$$(id -g)" \
		-e GOPATH=/work/.cache/gopath \
		-e GOMODCACHE=/work/.cache/gopath/pkg/mod \
		-e GOCACHE=/work/.cache/gocache \
		-e HOME=/tmp \
		-v "$(PWD):/work" \
		-w /work \
		app \
		go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.1 \
		-config api/oapi-codegen.types.yaml \
		api/swagger.yml
	$(COMPOSE) run --rm --no-deps \
		--user "$$(id -u):$$(id -g)" \
		-e GOPATH=/work/.cache/gopath \
		-e GOMODCACHE=/work/.cache/gopath/pkg/mod \
		-e GOCACHE=/work/.cache/gocache \
		-e HOME=/tmp \
		-v "$(PWD):/work" \
		-w /work \
		app \
		go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.1 \
		-config api/oapi-codegen.server.yaml \
		api/swagger.yml
	$(COMPOSE) run --rm --no-deps \
		--user "$$(id -u):$$(id -g)" \
		-e GOPATH=/work/.cache/gopath \
		-e GOMODCACHE=/work/.cache/gopath/pkg/mod \
		-e GOCACHE=/work/.cache/gocache \
		-e HOME=/tmp \
		-v "$(PWD):/work" \
		-w /work \
		app \
		go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.5.1 \
		-config api/oapi-codegen.spec.yaml \
		api/swagger.yml
