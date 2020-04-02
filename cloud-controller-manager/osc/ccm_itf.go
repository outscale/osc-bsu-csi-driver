/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package osc

// ********************* CCM Used Interfaces *********************

// Services is an abstraction over AWS, to allow mocking/other implementations
type Services interface {
	Compute(region string) (EC2, error)
	LoadBalancing(region string) (ELB, error)
	LoadBalancingV2(region string) (ELBV2, error)
	Autoscaling(region string) (ASG, error)
	Metadata() (EC2Metadata, error)
	KeyManagement(region string) (KMS, error)
}
