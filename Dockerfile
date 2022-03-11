FROM debian:bullseye-20220228
RUN apt-get -y update && \
    apt-get -y install libc-bin=2.31-13+deb11u2 \
                       ca-certificates=20210119 \
                       e2fsprogs=1.46.2-2 mount=2.36.1-8+deb11u1 \
                       udev=247.3-6 util-linux=2.36.1-8+deb11u1 \
                       xfsprogs=5.10.0-4 --no-install-recommends && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY ./bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver

ENTRYPOINT ["/bin/aws-ebs-csi-driver"]
