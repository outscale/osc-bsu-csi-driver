ARG GOLANG_IMAGE_TAG=1.23.4-bookworm
ARG RUNTIME_IMAGE_TAG=3.18

# Build image
FROM golang:${GOLANG_IMAGE_TAG} AS builder

# This build arg is the version to embed in the CPI binary
ARG VERSION=${VERSION}

# This build arg controls the GOPROXY setting
ARG GOPROXY

WORKDIR /build
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org}
RUN make build

# Final IMAGE
FROM alpine:${RUNTIME_IMAGE_TAG}
RUN apk add --no-cache \
            ca-certificates=20241121-r1 \
            e2fsprogs=1.47.0-r2 \
            e2fsprogs-extra=1.47.0-r2 \
            xfsprogs=6.2.0-r2 \
            xfsprogs-extra=6.2.0-r2 \
            blkid=2.38.1-r8 \
            cryptsetup=2.6.1-r3
COPY  --from=builder /build/bin/osc-bsu-csi-driver /bin/osc-bsu-csi-driver

ENTRYPOINT ["/bin/osc-bsu-csi-driver"]
