FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY services/event-api/go.mod services/event-api/go.sum ./
RUN go mod download

COPY services/event-api/ ./

RUN mkdir -p /out && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/consumer ./cmd/consumer

# API runtime
FROM alpine:3.22 AS api

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /out/server ./app

EXPOSE 8080

ENTRYPOINT ["./app"]

# Consumer runtime
FROM alpine:3.22 AS consumer

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /out/consumer ./app

ENTRYPOINT ["./app"]
