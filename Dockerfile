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
COPY api/     ./api/
COPY db/      ./db/
COPY auth/    ./auth/
COPY email/   ./email/
COPY games/   ./games/
COPY offline/ ./offline/
COPY notify/  ./notify/
# Desktop sidecar mode (`--desktop`); compiled in but unused when hosted.
COPY desktop/ ./desktop/


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
# Alpine ships /bin/sh and ca-certificates out of the box. curl is added
# because Coolify's HTTP-type healthcheck renders a `curl` command inside the
# container (it does not use wget for that type); without it the check fails
# with `sh: curl: not found`. Static Go binary so libc is unused.
# ---------------------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl && \
    addgroup -S nonroot && adduser -S -G nonroot nonroot

# Migrations are embedded in the binary via //go:embed, so no extra COPY needed.
COPY --from=builder /app/server /server

# Document the port; actual binding is controlled by the PORT env var.
EXPOSE 8080

USER nonroot

ENTRYPOINT ["/server"]
