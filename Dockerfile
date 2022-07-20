ARG GOLANG_IMAGE_TAG=1.17.6-buster
ARG RUNTIME_IMAGE_TAG=bullseye-20220711

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
FROM debian:${RUNTIME_IMAGE_TAG}
RUN apt-get -y update && \
    apt-get -y install libc-bin=2.31-13+deb11u3 \
                       ca-certificates=20210119 \
                       e2fsprogs=1.46.2-2 \
                       mount=2.36.1-8+deb11u1 \
                       util-linux=2.36.1-8+deb11u1 \
                       udev=247.3-7 \
                       xfsprogs=5.10.0-4 \
                       gzip=1.10-4+deb11u1 \
                       liblzma5=5.2.5-2.1~deb11u1 \
                       --no-install-recommends && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY  --from=builder /build/bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver

ENTRYPOINT ["/bin/aws-ebs-csi-driver"]
