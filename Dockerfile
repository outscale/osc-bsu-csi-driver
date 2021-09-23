FROM debian:stable-20210902
RUN echo "deb http://deb.debian.org/debian testing non-free contrib main" >> /etc/apt/sources.list &&\
    echo "deb http://deb.debian.org/debian unstable non-free contrib main" >> /etc/apt/sources.list && \
    apt-get -y update && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN apt-get -y update && \
	apt-get -y install libc-bin=2.32-4 \
						ca-certificates=20210119 \
						e2fsprogs=1.46.2-2 mount=2.37.2-3 \
						udev=247.9-2 util-linux=2.37.2-3 \
						xfsprogs=5.10.0-4 --no-install-recommends && \
	apt-get clean && rm -rf /var/lib/apt/lists/*

COPY ./bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver

ENTRYPOINT ["/bin/aws-ebs-csi-driver"]
