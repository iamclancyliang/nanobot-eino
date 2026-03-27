# Multi-stage build for Nanobot Eino
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o nanobot-eino ./cmd/nanobot/main.go

FROM alpine:latest
WORKDIR /root
COPY --from=builder /app/nanobot-eino /usr/local/bin/nanobot-eino
RUN mkdir -p /root/.nanobot-eino
VOLUME /root/.nanobot-eino

ENTRYPOINT ["nanobot-eino"]
CMD ["version"]
