# --- STAGE 1: Builder ---
FROM golang:1.24.6-alpine AS builder

RUN apk --no-cache add build-base libstdc++ sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o forum -ldflags="-s -w" ./cmd/server/main.go

# --- STAGE 2: Runner (Minimal Image) ---
FROM alpine:latest
RUN apk --no-cache add sqlite-libs libstdc++

WORKDIR /forum

COPY --from=builder /app/forum .
COPY --from=builder /app/assets/ ./assets/

RUN mkdir -p /forum/src

EXPOSE 80
    
CMD ["./forum"]