FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN go install github.com/githubnemo/CompileDaemon@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENTRYPOINT ["CompileDaemon", "-build=go build -o main .", "-command=./main", "-polling"]

