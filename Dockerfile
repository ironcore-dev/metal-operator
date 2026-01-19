# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.26rc2 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/manager/main.go cmd/manager/main.go
COPY cmd/metalprobe/main.go cmd/metalprobe/main.go
COPY api/ api/
COPY internal/ internal/
COPY bmc/ bmc/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
FROM builder AS manager-builder
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/manager/main.go

FROM builder AS probe-builder
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o metalprobe cmd/metalprobe/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot AS manager
LABEL source_repository="https://github.com/ironcore-dev/metal-operator"
WORKDIR /
COPY --from=manager-builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]

FROM debian:testing-slim AS probe
LABEL source_repository="https://github.com/ironcore-dev/metal-operator"
WORKDIR /
COPY --from=probe-builder /workspace/metalprobe .
COPY hack/metalprobe_launch.sh /launch.sh
RUN chmod +x /launch.sh
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates bash curl iproute2 iputils-ping net-tools ethtool lldpd && \
    rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["/launch.sh"]
