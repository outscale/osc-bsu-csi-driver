ARG GOLANG_IMAGE_TAG=1.23.7-bookworm
# Tools are taken from Debian 12
ARG TOOLS_IMAGE_TAG=12
# Distroless debug is used to get a busybox shell
ARG RUNTIME_IMAGE_TAG=debug-dca9008b864a381b5ce97196a4d8399ac3c2fa65

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

# Source image for all fs binaries (mkfs, mount/umount, fsck, ...)
FROM debian:${TOOLS_IMAGE_TAG} AS debian

RUN apt-get update && apt-get install -y --no-install-recommends util-linux e2fsprogs mount xfsprogs cryptsetup-bin

# Scratch image with all binaries & libs
FROM scratch AS tmp

COPY --from=builder /build/bin/osc-bsu-csi-driver /bin/osc-bsu-csi-driver
COPY --from=debian /bin/sh \
        /bin/mount \
        /bin/umount /bin/
COPY --from=debian /sbin/blkid \
        /sbin/blockdev \
        /sbin/cryptsetup \
        /sbin/dumpe2fs \
        /sbin/e2freefrag \
        /sbin/e2fsck \
        /sbin/e2image \
        /sbin/e2mmpstatus \
        /sbin/e2undo \
        /sbin/fsck \
        /sbin/fsck.ext2 \
        /sbin/fsck.ext3 \
        /sbin/fsck.ext4 \
        /sbin/fsck.xfs \
        /sbin/mke2fs \
        /sbin/mkfs \
        /sbin/mkfs.ext2 \
        /sbin/mkfs.ext3 \
        /sbin/mkfs.ext4 \
        /sbin/mkfs.xfs \
        /sbin/resize2fs \
        /sbin/xfs_repair /sbin/
COPY --from=debian /lib/x86_64-linux-gnu/libargon2.so.1 \
        /lib/x86_64-linux-gnu/libblkid.so.1 \
        /lib/x86_64-linux-gnu/libc.so.6 \
        /lib/x86_64-linux-gnu/libcom_err.so.2 \
        /lib/x86_64-linux-gnu/libcrypto.so.3 \
        /lib/x86_64-linux-gnu/libcryptsetup.so.12 \
        /lib/x86_64-linux-gnu/libdevmapper.so.1.02.1 \
        /lib/x86_64-linux-gnu/libe2p.so.2 \
        /lib/x86_64-linux-gnu/libext2fs.so.2 \
        /lib/x86_64-linux-gnu/libinih.so.1 \
        /lib/x86_64-linux-gnu/libjson-c.so.5 \
        /lib/x86_64-linux-gnu/libm.so.6 \
        /lib/x86_64-linux-gnu/libmount.so.1 \
        /lib/x86_64-linux-gnu/libpcre2-8.so.0 \
        /lib/x86_64-linux-gnu/libpopt.so.0 \
        /lib/x86_64-linux-gnu/libselinux.so.1 \
        /lib/x86_64-linux-gnu/libtinfo.so.6 \
        /lib/x86_64-linux-gnu/libudev.so.1 \
        /lib/x86_64-linux-gnu/liburcu.so.8 \
        /lib/x86_64-linux-gnu/libuuid.so.1 \
        /lib/x86_64-linux-gnu/libz.so.1 \
        /lib/x86_64-linux-gnu/libgcc_s.so.1 /lib/x86_64-linux-gnu/
COPY --from=debian /lib64/ld-linux-x86-64.so.2 /lib64/
COPY --from=debian /etc/mke2fs.conf /etc/

# Final IMAGE
FROM gcr.io/distroless/static-debian12:${RUNTIME_IMAGE_TAG}
COPY --from=tmp / /

ENTRYPOINT ["/bin/osc-bsu-csi-driver"]
