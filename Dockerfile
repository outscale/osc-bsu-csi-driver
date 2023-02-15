ARG GOLANG_IMAGE_TAG=1.19-buster
ARG RUNTIME_IMAGE_TAG=3.13.12

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
            ca-certificates=20220614-r0 \
            e2fsprogs=1.45.7-r0 \
            xfsprogs=5.10.0-r0 \
            blkid=2.37.4-r0 \
            e2fsprogs-extra=1.45.7-r0 \
            cryptsetup=2.3.7-r0
COPY  --from=builder /build/bin/osc-bsu-csi-driver /bin/osc-bsu-csi-driver

ENTRYPOINT ["/bin/osc-bsu-csi-driver"]
