# Contexto de build: raíz del repositorio (necesita games/).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY ingest/go.mod ./
RUN go mod download
COPY ingest/ ./
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ingest .

FROM alpine:3.20
RUN adduser -D -H app && mkdir -p /data && chown app /data
WORKDIR /app
COPY --from=build /out/ingest /app/ingest
COPY games /app/games
ENV ADDR=:8080 GAMES_DIR=/app/games DATA_DIR=/data
USER app
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
  CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["/app/ingest"]
