# syntax=docker/dockerfile:1
# Build context: repository root (the image bundles flights.json).
#   docker build -f backend/Dockerfile .

FROM golang:1.26-alpine AS build
WORKDIR /src/backend

# Copy the module files first so dependency downloads are cached separately
# from source changes.
COPY backend/go.mod backend/go.sum* ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.24
RUN addgroup -S -g 10001 skypath \
 && adduser -S -u 10001 -G skypath -H -h /nonexistent -s /sbin/nologin skypath

COPY --from=build /out/server /app/server
COPY flights.json /data/flights.json

ENV PORT=8080 \
    FLIGHTS_PATH=/data/flights.json

USER skypath:skypath
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/server", "-healthcheck"]

ENTRYPOINT ["/app/server"]
