# v0.0.6beta
## Notable changes
* Enable API sdk logs 

# v0.0.7beta
## Notable changes
* Use [osc-sdk-go](https://github.com/outscale/osc-sdk-go) instead of aws-sdk-go
* Update to helm3
* Update of sidecar images

# v0.0.8beta
## Notable changes
* Implement NodeGetVolumeStats


# v0.0.9beta
## Notable changes
* Implement ControllerExpandVolume using UpdateVolume api call
* Fix regression in detach disk toleration
* customise sidecars conatiner verbosity and timeout

# v0.0.10beta
## Notable changes
* Add fsGroupPolicy field to CSIDriver object and customize it with chart values

# v0.0.11beta
## Notable changes
* Add fsGroupPolicy specific e2e test
