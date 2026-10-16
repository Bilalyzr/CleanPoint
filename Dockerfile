# CleanPoint — optional single-process container (local simulation only).
# Build:  docker build -t cleanpoint:local .
# Run:    docker run -p 127.0.0.1:8080:8080 -v cleanpoint-data:/app/sandbox cleanpoint:local
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cleanpoint-server ./cmd/server \
 && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/cleanpoint ./cmd/cli

FROM python:3.12-slim
WORKDIR /app
COPY --from=build /out/ /app/bin/
COPY simulator/ /app/simulator/
COPY benchmarks/ /app/benchmarks/
COPY fixtures/ /app/fixtures/
ENV CLEANPOINT_ADDR=0.0.0.0:8080 \
    CLEANPOINT_SANDBOX_ROOT=/app/sandbox \
    PATH="/app/bin:${PATH}"
# loopback-only exposure is the operator's responsibility; the image defaults
# to binding all interfaces INSIDE the container, which is still local-only
# when published as 127.0.0.1:8080:8080.
EXPOSE 8080
CMD ["/app/bin/cleanpoint-server"]
