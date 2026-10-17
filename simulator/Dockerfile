# Contexto de build: raíz del repositorio (los tests leen games/).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY games /games
COPY simulator/go.mod ./simulator/
RUN cd simulator && go mod download
COPY simulator/ ./simulator/
RUN cd simulator && ln -s /games ../games && go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/simulator .

FROM alpine:3.20
RUN adduser -D -H app
COPY --from=build /out/simulator /app/simulator
ENV TARGET=http://ingest:8080/events
USER app
ENTRYPOINT ["/app/simulator"]
