# HTTP Task Runner

Сервис принимает HTTP-задачи, выполняет внешние запросы в фоне и позволяет получать статус выполнения задач.

## Features

- `POST /task` создание задачи
- `GET /task/{id}` получение статуса/результата задачи
- `GET /tasks` получение списка всех задач
- `DELETE /task/{id}` удаление задачи
- `GET /healthz` проверка состояния сервиса
- `GET /swagger` Swagger
- `GET /swagger/index.html` Swagger UI

## Конфигурация

Скопируйте `.env.example` в `.env` и при необходимости измените значения.

Основные переменные:

- `HTTP_ADDR`
- `HTTP_PORT`
- `DATABASE_URL`
- `POSTGRES_HOST_PORT`
- `HTTP_EXECUTOR_TIMEOUT`
- `HTTP_SERVER_READ_TIMEOUT`
- `HTTP_SERVER_WRITE_TIMEOUT`
- `HTTP_SERVER_IDLE_TIMEOUT`
- `HTTP_SHUTDOWN_TIMEOUT`
- `LOG_LEVEL`
- `LOG_FORMAT`

## Запуск локально

1. Поднимите контейнеры (приложение + БД):

```bash
make up
```

2. Проверить состояние контейнеров:

```bash
make ps
```

## Миграции

При старте приложения миграции применяются автоматически через GORM `AutoMigrate`.

Дополнительно доступны команды:

- Применить вручную: `make migrate-up`
- Откатить (удаляет таблицу `tasks`): `make migrate-down`

## Docker

Собрать образ:

```bash
make docker-build
```

Запустить приложение и БД:

```bash
make up
```

Остановить:

```bash
make down
```

## Примеры API

Создать задачу:

```bash
curl -s -X POST http://localhost:9091/task \
  -H "Content-Type: application/json" \
  -d '{
    "method": "GET",
    "url": "https://httpbin.org/get",
    "headers": {
      "Authorization": "Test 4"
    }
  }'
```

Получить задачу по id:

```bash
curl -s http://localhost:9091/task/<TASK_ID>
```

Получить все задачи:

```bash
curl -s http://localhost:9091/tasks
```

Проверка health:

```bash
curl -s http://localhost:9091/healthz
```

Swagger UI:

```bash
curl -s http://localhost:9091/swagger/index.html
```

## Тесты

```bash
make test
```

Команды:

```bash
# unit / mock тесты
make test-unit

# все интеграционные тесты
make test-integration

# только интеграционные тесты store
make test-integration-store

# только интеграционные тесты HTTP router
make test-integration-http

# unit + integration
make test-all
```

## OAPI Codegen

Источник OpenAPI схемы:
- `api/swagger.yml`

Генерация Go-кода (models + server + embedded spec):

```bash
make oapi-gen
```

Сгенерированные файлы появятся в:
- `internal/delivery/http/openapi/types.gen.go`
- `internal/delivery/http/openapi/server.gen.go`
- `internal/delivery/http/openapi/spec.gen.go`
