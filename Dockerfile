# --- Build ---
FROM golang:1.25.14-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/pet-service ./cmd/server

# --- Run ---
# distroless แทน scratch: ได้ /etc/passwd, ca-certificates, tzdata และ user nonroot มาให้
# ของเดิมใช้ scratch + WORKDIR /root/ ซึ่งรันเป็น UID 0 (S-7)
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/pet-service /app/pet-service
# public key ยังติดมากับ image เพื่อความเข้ากันได้
# แต่ควรย้ายไปใช้ JWT_PUBLIC_KEY จาก Secret เพื่อให้ rotate ได้โดยไม่ต้อง rebuild (S-6)
COPY keys /app/keys

WORKDIR /app
USER nonroot:nonroot
EXPOSE 4001

ENTRYPOINT ["/app/pet-service"]
