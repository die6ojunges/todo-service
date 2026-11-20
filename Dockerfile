# syntax=docker/dockerfile:1

# Build stage: the official Go image (challenge requirement). Same minor as go.mod.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/ cmd/
COPY internal/ internal/
# Cache mounts keep modules and build cache between builds without baking them into a layer.
# CGO off gives a static binary that runs on the shell-less runtime image below.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/todo-api ./cmd/todo-api

# Runtime stage: distroless static has no shell or package manager (small attack surface),
# but does have CA certificates and a non-root user. Only the binary is copied over.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/todo-api /todo-api
USER nonroot:nonroot
EXPOSE 8080
# Exec form, so the binary is PID 1 and receives SIGTERM directly for graceful shutdown.
ENTRYPOINT ["/todo-api"]
