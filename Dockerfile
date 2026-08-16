# Build Stage
FROM golang:1.25.1-alpine AS builder
RUN apk update && apk add --no-cache git ca-certificates tzdata
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o main ./cmd/server

# Run Stage
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
WORKDIR /root/
COPY --from=builder /app/main .
COPY keys ./keys
EXPOSE 4001
CMD ["./main"]
