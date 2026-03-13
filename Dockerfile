# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1 — Build
# Uses the full Go toolchain image; the workspace (go.work) covers all three
# modules (api/, db/, auth/).
# ---------------------------------------------------------------------------
FROM golang:1.24-bookworm AS builder

WORKDIR /build

# Copy workspace manifests first so Docker can cache the dependency download
# layer independently of source changes.
COPY go.work go.work.sum ./
COPY api/go.mod  api/go.sum  ./api/
COPY db/go.mod   db/go.sum   ./db/

# Download all module dependencies (cached unless go.mod/go.sum change).
RUN go work sync && \
    cd api  && go mod download && \
    cd ../db && go mod download

# Copy the rest of the source.
COPY api/  ./api/
COPY db/   ./db/


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
# distroless/static contains only root certificates and timezone data —
# no shell, no package manager, minimal attack surface.
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

# Migrations are embedded in the binary via //go:embed, so no extra COPY needed.
COPY --from=builder /app/server /server

# Document the port; actual binding is controlled by the PORT env var.
EXPOSE 8080

# Run as the non-root user provided by the distroless image (uid 65532).
USER nonroot

ENTRYPOINT ["/server"]
