FROM golang:1.24-bookworm

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/http-task-runner ./cmd/server

EXPOSE 9091

CMD ["/app/http-task-runner"]
