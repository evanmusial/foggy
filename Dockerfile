# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package.json web/tsconfig.json web/vite.config.ts ./
COPY web/package-lock.json ./
COPY web/index.html ./index.html
COPY web/src ./src
RUN npm ci
RUN npm run build

FROM golang:1.25-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/cortex ./cmd/cortex
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cortex-healthcheck ./cmd/cortex-healthcheck

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home --home-dir /app cortex \
  && mkdir -p /app/web/dist /data \
  && chown -R cortex:cortex /app /data
WORKDIR /app
COPY --from=backend /out/cortex /app/cortex
COPY --from=backend /out/cortex-healthcheck /app/cortex-healthcheck
COPY --from=frontend /src/web/dist /app/web/dist
ENV CORTEX_ADDR=:8080
ENV CORTEX_DATA_DIR=/data
ENV CORTEX_ORIGIN=http://localhost:8080
ENV CORTEX_RP_ID=localhost
ENV CORTEX_REQUIRE_HTTPS=true
USER cortex
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD ["/app/cortex-healthcheck"]
CMD ["/app/cortex"]
