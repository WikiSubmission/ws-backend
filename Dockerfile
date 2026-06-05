# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1 — Build
# ---------------------------------------------------------------------------
FROM golang:1.26-bookworm AS builder

WORKDIR /build

# Copy module manifests first so Docker can cache the dependency download
# layer independently of source changes.
COPY go.mod go.sum ./

# Download all module dependencies (cached unless go.mod/go.sum change).
RUN go mod download

# Copy the rest of the source.
COPY api/    ./api/
COPY db/     ./db/
COPY auth/   ./auth/
COPY sanity/ ./sanity/
COPY games/  ./games/


# Produce a fully static binary (no CGO, no libc dependency).
# -trimpath removes local file paths from stack traces.
# -w -s strip DWARF debug info and symbol table for a smaller image.
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags="-w -s" \
      -o /app/server \
      ./api/

# ---------------------------------------------------------------------------
# Stage 2 — Runtime
# Alpine ships /bin/sh, wget, and ca-certificates out of the box so platform
# healthchecks (Coolify, Kubernetes) that wrap calls in `sh -c "wget ..."`
# work without extra plumbing. Static Go binary so libc is unused.
# ---------------------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S nonroot && adduser -S -G nonroot nonroot

# Migrations are embedded in the binary via //go:embed, so no extra COPY needed.
COPY --from=builder /app/server /server

# Document the port; actual binding is controlled by the PORT env var.
EXPOSE 8080

USER nonroot

ENTRYPOINT ["/server"]
