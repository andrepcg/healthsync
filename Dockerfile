# syntax=docker/dockerfile:1

# ---- 1. web UI -------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# vite.config.ts writes to ../internal/web/dist
RUN mkdir -p /src/internal/web && npm run build

# ---- 2. Go binary ----------------------------------------------------------
FROM golang:1.26-alpine AS build
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/healthsync .

# ---- 3. runtime ------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -g 1000 healthsync \
 && adduser -D -u 1000 -G healthsync healthsync \
 && mkdir -p /data && chown healthsync:healthsync /data
COPY --from=build /out/healthsync /usr/local/bin/healthsync
ENV HEALTHSYNC_DATA_DIR=/data \
    HEALTHSYNC_NO_UPDATE_CHECK=1 \
    TZ=Europe/Lisbon
USER healthsync
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/healthz >/dev/null || exit 1
ENTRYPOINT ["healthsync", "server", "--host", "0.0.0.0", "--port", "8080"]
