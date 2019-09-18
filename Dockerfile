# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.12.7-stretch as builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver
ADD . .
RUN apt-get -y update && \
    apt-get -y install gdb && \
    echo "add-auto-load-safe-path /usr/local/go/src/runtime/runtime-gdb.py" >> /root/.gdbinit && \
    make && \
    cp /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver/bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver

ENTRYPOINT ["/bin/aws-ebs-csi-driver"]

#FROM amazonlinux:2
#RUN yum install ca-certificates e2fsprogs xfsprogs util-linux -y
#COPY --from=builder /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver/bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver
#RUN cp /go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver/bin/aws-ebs-csi-driver /bin/aws-ebs-csi-driver
#ENTRYPOINT ["/bin/aws-ebs-csi-driver"]


#docker run -it  -v /home/anisz/poc-cloud-provider/aws-ebs-csi-driver:/go/src/github.com/kubernetes-sigs/aws-ebs-csi-driver  \
#                -v /home/anisz/poc-cloud-provider/pkg:/go/pkg/  \
#                -v /home/anisz/poc-cloud-provider/aws-ebs-csi-driver/.aws/:/root/.aws  \
#                --cap-add=SYS_PTRACE --security-opt seccomp=unconfined \
#                 --name="TEST-aws-ebs-csi-driver-osc" --rm aws-ebs-csi-driver-osc:latest