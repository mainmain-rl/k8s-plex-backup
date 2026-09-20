# Building stage
FROM golang:1.26.0-alpine AS builder

WORKDIR /build

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download
RUN go mod verify

# Code copy
COPY internal/. internal/.
COPY cmd/. cmd/.

RUN go mod download 2>/dev/null || true

# TARGETOS/TARGETARCH sont fournis automatiquement par buildx pour chaque
# plateforme cible listée dans `platforms:` du workflow (amd64, arm64, ...).
ARG TARGETOS
ARG TARGETARCH

# Build the application
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/k8s-plex-backup ./cmd/k8s-plex-backup


# Final Stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/k8s-plex-backup /k8s-plex-backup
USER nonroot:nonroot
ENTRYPOINT ["/k8s-plex-backup"]
