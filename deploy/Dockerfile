# ── Stage 1: Build dashboard ─────────────────────────────────────────────────
FROM node:20-alpine AS dashboard
WORKDIR /app
COPY web/dashboard/package.json web/dashboard/package-lock.json* ./
RUN npm install --silent
COPY web/dashboard/ .
RUN npm run build

# ── Stage 2: Build Go binary ────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=dashboard /app/dist web/dashboard/dist/
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /fleet-server \
    ./cmd/fleet-server/

# ── Stage 3: Runtime ────────────────────────────────────────────────────────
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -H -s /sbin/nologin fibratus

COPY --from=builder /fleet-server /usr/local/bin/fleet-server
COPY configs/fleet-server.yml /etc/fibratus/fleet-server.yml

USER fibratus
EXPOSE 8443

ENTRYPOINT ["fleet-server"]
CMD ["serve", "--config", "/etc/fibratus/fleet-server.yml"]
