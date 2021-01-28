## E2E Testing
E2E test verifies the funcitonality of BSU CSI driver in the context of Kubernetes. It exercises driver feature e2e including static provisioning, dynamic provisioning, volume scheduling, mount options, etc.



By default `make test-e2e-single-az` targets will run 32 tests concurrently, set `GINKGO_NODES` to change the parallelism.


