# ---- Build stage for the Go backend ----
FROM golang:1.22-alpine AS backend-build
RUN apk add --no-cache build-base
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go build -o /out/fiber-monitor ./cmd/server

# ---- Build stage for the Vue frontend ----
FROM node:20-alpine AS frontend-build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# ---- Runtime image ----
FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates iproute2 iputils-ping xl2tpd sstp-client \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/web
WORKDIR /app

COPY --from=backend-build /out/fiber-monitor /app/fiber-monitor
COPY --from=frontend-build /app/dist /app/web

ENV SERVER_PORT=8080
EXPOSE 8080

# The container needs NET_ADMIN + root to manage VPN tunnels; run unprivileged
# ping via the icmp socket otherwise.
CMD ["/app/fiber-monitor"]