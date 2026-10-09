# Building stage
FROM golang:1.26.0-alpine AS builder

WORKDIR /build

# Install UPX (for binary compression) and CA certs
RUN apk add --no-cache upx ca-certificates tzdata

# Create a non-root user to use in the final scratch image
RUN adduser -D -g '' -u 65532 nonroot

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Code copy
COPY internal/. internal/.
COPY cmd/. cmd/.

ARG TARGETOS
ARG TARGETARCH

# Build the application
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/k8s-plex-backup ./cmd/k8s-plex-backup

# Compress the binary to reduce its size by 50-70%
RUN upx --best --lzma /out/k8s-plex-backup

# Final Stage
FROM scratch

# Import timezone data, CA certs, and user details from the builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy the compressed binary
COPY --from=builder /out/k8s-plex-backup /k8s-plex-backup

USER nonroot:nonroot
ENTRYPOINT ["/k8s-plex-backup"]
