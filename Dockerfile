ARG GOLANG_IMAGE_TAG=1.17.6-buster
ARG RUNTIME_IMAGE_TAG=3.13

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
RUN apk add --no-cache ca-certificates e2fsprogs xfsprogs blkid findmnt e2fsprogs-extra xfsprogs-extra
COPY  --from=builder /build/bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver

ENTRYPOINT ["/bin/aws-ebs-csi-driver"]
