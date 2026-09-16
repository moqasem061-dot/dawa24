# ==============================================================================
# Dawa24 Store - production image (three binaries, one build)
# ==============================================================================
#
# Build speed notes (why this file is shaped the way it is):
#
#  - The builder image is pinned to a Go minor version. `golang:bookworm` is a
#    moving tag: when upstream ships a Go patch the digest changes, every cached
#    layer below it is invalidated and the whole tree recompiles from cold. A
#    pinned tag turns that from "every few days" into "when we choose to".
#
#  - `go mod download` is its own layer, keyed only on go.mod/go.sum, so it runs
#    again only when dependencies actually change.
#
#  - The build- and module-cache mounts survive between builds on a persistent
#    BuildKit daemon (which Elest.io's build host is). A warm cache turns a
#    full-tree recompile into an incremental one - only changed packages and
#    their dependents are rebuilt.
#
#  - .dockerignore keeps *_test.go, test/, data/, docs and *.md out of the
#    context, so a test-only or docs-only commit does not invalidate the
#    `COPY . .` layer and the build cache is reused as-is.
#
#  - The three binaries are built in one `go build` invocation so the dependency
#    graph is loaded and type-checked once, not three times.
#
#  - The runtime image copies only the binaries. Static assets and SQL
#    migrations are compiled into the binaries via //go:embed
#    (internal/ui/static, db/migrations), so there is nothing else to ship.

FROM golang:1.26-bookworm AS builder

WORKDIR /app

ENV CGO_ENABLED=0 \
    GOFLAGS=-buildvcs=false

# 1. Dependencies - re-runs only when go.mod / go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 2. Application source (see .dockerignore for what is deliberately excluded).
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown

# 3. Compile server, worker and cli in a single pass. `-o /out/` writes one
#    binary per named package. Both cache mounts persist across builds on a
#    long-lived BuildKit daemon.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath \
      -ldflags="-s -w -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT}" \
      -o /out/ \
      ./cmd/server ./cmd/worker ./cmd/cli

# ------------------------------------------------------------------------------
# Runtime - minimal Debian, binaries only.
# ------------------------------------------------------------------------------
FROM debian:bookworm-slim

WORKDIR /app

ENV TZ=Africa/Cairo

# ca-certificates for outbound TLS to the Gateway and object storage.
# curl for the container healthcheck.
# tzdata for Africa/Cairo timezone localization.
# This layer is keyed on the base image and this RUN line only, so it is reused
# on every application-code build.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl tzdata && \
    ln -fs /usr/share/zoneinfo/Africa/Cairo /etc/localtime && \
    echo "Africa/Cairo" > /etc/timezone && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r dawa24 && useradd -r -g dawa24 -d /app dawa24

COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/cli    /app/cli

# Create the uploads tree inside the image so that a freshly-created named
# volume mounted here inherits dawa24 ownership (Docker seeds an empty volume
# from the image path, permissions included). Without this the non-root process
# cannot write into a root-owned volume and every upload fails.
RUN mkdir -p /app/data/uploads/products \
             /app/data/uploads/licenses \
             /app/data/uploads/avatars \
             /app/data/uploads/resumes \
             /app/data/uploads/documents \
             /app/data/uploads/brands \
             /app/data/uploads/compare \
             /app/data/uploads/imports \
             /app/data/uploads/receipts && \
    chown -R dawa24:dawa24 /app

USER dawa24

EXPOSE 8080

CMD ["/app/server"]
