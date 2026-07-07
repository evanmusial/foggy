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
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/foggy ./cmd/foggy
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/foggy-healthcheck ./cmd/foggy-healthcheck

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home --home-dir /app foggy \
  && mkdir -p /app/web/dist /data \
  && chown -R foggy:foggy /app /data
WORKDIR /app
COPY --from=backend /out/foggy /app/foggy
COPY --from=backend /out/foggy-healthcheck /app/foggy-healthcheck
COPY --from=frontend /src/web/dist /app/web/dist
ENV FOGGY_ADDR=:8080
ENV FOGGY_DATA_DIR=/data
ENV FOGGY_ORIGIN=http://localhost:8080
ENV FOGGY_RP_ID=localhost
ENV FOGGY_REQUIRE_HTTPS=true
USER foggy
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD ["/app/foggy-healthcheck"]
CMD ["/app/foggy"]
