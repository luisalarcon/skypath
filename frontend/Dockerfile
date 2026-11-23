# syntax=docker/dockerfile:1
# Build context: ./frontend
#   docker build -f frontend/Dockerfile frontend

FROM golang:1.26-alpine AS build
WORKDIR /src

# Copy the module files first so dependency downloads are cached separately
# from source changes.
COPY go.mod go.sum* ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/web ./cmd/web

FROM alpine:3.24
RUN addgroup -S -g 10001 skypath \
 && adduser -S -u 10001 -G skypath -H -h /nonexistent -s /sbin/nologin skypath

COPY --from=build /out/web /app/web

ENV PORT=3000

USER skypath:skypath
EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/web", "-healthcheck"]

ENTRYPOINT ["/app/web"]
