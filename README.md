# HTTP Task Runner

Сервис принимает HTTP-задачи, выполняет внешние запросы в фоне и позволяет получать статус выполнения задач.

## Features

- `POST /task` создание задачи
- `GET /task/{id}` получение статуса/результата задачи
- `GET /tasks` получение списка всех задач
- `DELETE /task/{id}` удаление задачи
- `GET /healthz` проверка состояния сервиса

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

## Запуск локально

1. Поднимите контейнеры (приложение + БД):

```bash
make up
```

2. Примените миграции (вручную, не на старте приложения):

```bash
make migrate-up
```

3. Проверить состояние контейнеров:

```bash
make ps
```

## Миграции

Применить:

```bash
make migrate-up
```

Откатить последнюю примененную:

```bash
make migrate-down
```

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

## Тесты

```bash
make test
```
