FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /activity-watcher ./cmd/activity-watcher

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /activity-watcher .
VOLUME ["/data"]
ENV DB_PATH=/data/events.db
ENV PORT=8080
ENV LOG_LEVEL=info
EXPOSE 8080
ENTRYPOINT ["/app/activity-watcher"]
