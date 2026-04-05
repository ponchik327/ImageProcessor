FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /image-processor ./cmd/app

# ── runtime ──────────────────────────────────────────────────────────────────
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /image-processor .
COPY --from=builder /app/web ./web
COPY --from=builder /app/assets ./assets
EXPOSE 8080
ENTRYPOINT ["/app/image-processor"]
