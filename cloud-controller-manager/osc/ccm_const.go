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

import (
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ********************* CCM Const Definition *********************

// NLBHealthCheckRuleDescription is the comment used on a security group rule to
// indicate that it is used for health checks
const NLBHealthCheckRuleDescription = "kubernetes.io/rule/nlb/health"

// NLBClientRuleDescription is the comment used on a security group rule to
// indicate that it is used for client traffic
const NLBClientRuleDescription = "kubernetes.io/rule/nlb/client"

// NLBMtuDiscoveryRuleDescription is the comment used on a security group rule
// to indicate that it is used for mtu discovery
const NLBMtuDiscoveryRuleDescription = "kubernetes.io/rule/nlb/mtu"

// ProviderName is the name of this cloud provider.
const ProviderName = "aws"

// TagNameKubernetesService is the tag name we use to differentiate multiple
// services. Used currently for ELBs only.
const TagNameKubernetesService = "kubernetes.io/service-name"

// TagNameSubnetInternalELB is the tag name used on a subnet to designate that
// it should be used for internal ELBs
const TagNameSubnetInternalELB = "kubernetes.io/role/internal-elb"

// TagNameSubnetPublicELB is the tag name used on a subnet to designate that
// it should be used for internet ELBs
const TagNameSubnetPublicELB = "kubernetes.io/role/elb"

// ServiceAnnotationLoadBalancerType is the annotation used on the service
// to indicate what type of Load Balancer we want. Right now, the only accepted
// value is "nlb"
const ServiceAnnotationLoadBalancerType = "service.beta.kubernetes.io/aws-load-balancer-type"

// ServiceAnnotationLoadBalancerInternal is the annotation used on the service
// to indicate that we want an internal ELB.
const ServiceAnnotationLoadBalancerInternal = "service.beta.kubernetes.io/aws-load-balancer-internal"

// ServiceAnnotationLoadBalancerProxyProtocol is the annotation used on the
// service to enable the proxy protocol on an ELB. Right now we only accept the
// value "*" which means enable the proxy protocol on all ELB backends. In the
// future we could adjust this to allow setting the proxy protocol only on
// certain backends.
const ServiceAnnotationLoadBalancerProxyProtocol = "service.beta.kubernetes.io/aws-load-balancer-proxy-protocol"

// ServiceAnnotationLoadBalancerAccessLogEmitInterval is the annotation used to
// specify access log emit interval.
const ServiceAnnotationLoadBalancerAccessLogEmitInterval = "service.beta.kubernetes.io/aws-load-balancer-access-log-emit-interval"

// ServiceAnnotationLoadBalancerAccessLogEnabled is the annotation used on the
// service to enable or disable access logs.
const ServiceAnnotationLoadBalancerAccessLogEnabled = "service.beta.kubernetes.io/aws-load-balancer-access-log-enabled"

// ServiceAnnotationLoadBalancerAccessLogS3BucketName is the annotation used to
// specify access log s3 bucket name.
const ServiceAnnotationLoadBalancerAccessLogS3BucketName = "service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name"

// ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix is the annotation used
// to specify access log s3 bucket prefix.
const ServiceAnnotationLoadBalancerAccessLogS3BucketPrefix = "service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix"

// ServiceAnnotationLoadBalancerConnectionDrainingEnabled is the annnotation
// used on the service to enable or disable connection draining.
const ServiceAnnotationLoadBalancerConnectionDrainingEnabled = "service.beta.kubernetes.io/aws-load-balancer-connection-draining-enabled"

// ServiceAnnotationLoadBalancerConnectionDrainingTimeout is the annotation
// used on the service to specify a connection draining timeout.
const ServiceAnnotationLoadBalancerConnectionDrainingTimeout = "service.beta.kubernetes.io/aws-load-balancer-connection-draining-timeout"

// ServiceAnnotationLoadBalancerConnectionIdleTimeout is the annotation used
// on the service to specify the idle connection timeout.
const ServiceAnnotationLoadBalancerConnectionIdleTimeout = "service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout"

// ServiceAnnotationLoadBalancerCrossZoneLoadBalancingEnabled is the annotation
// used on the service to enable or disable cross-zone load balancing.
const ServiceAnnotationLoadBalancerCrossZoneLoadBalancingEnabled = "service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled"

// ServiceAnnotationLoadBalancerExtraSecurityGroups is the annotation used
// on the service to specify additional security groups to be added to ELB created
const ServiceAnnotationLoadBalancerExtraSecurityGroups = "service.beta.kubernetes.io/aws-load-balancer-extra-security-groups"

// ServiceAnnotationLoadBalancerSecurityGroups is the annotation used
// on the service to specify the security groups to be added to ELB created. Differently from the annotation
// "service.beta.kubernetes.io/aws-load-balancer-extra-security-groups", this replaces all other security groups previously assigned to the ELB.
const ServiceAnnotationLoadBalancerSecurityGroups = "service.beta.kubernetes.io/aws-load-balancer-security-groups"

// ServiceAnnotationLoadBalancerCertificate is the annotation used on the
// service to request a secure listener. Value is a valid certificate ARN.
// For more, see http://docs.aws.amazon.com/ElasticLoadBalancing/latest/DeveloperGuide/elb-listener-config.html
// CertARN is an IAM or CM certificate ARN, e.g. arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012
const ServiceAnnotationLoadBalancerCertificate = "service.beta.kubernetes.io/aws-load-balancer-ssl-cert"

// ServiceAnnotationLoadBalancerSSLPorts is the annotation used on the service
// to specify a comma-separated list of ports that will use SSL/HTTPS
// listeners. Defaults to '*' (all).
const ServiceAnnotationLoadBalancerSSLPorts = "service.beta.kubernetes.io/aws-load-balancer-ssl-ports"

// ServiceAnnotationLoadBalancerSSLNegotiationPolicy is the annotation used on
// the service to specify a SSL negotiation settings for the HTTPS/SSL listeners
// of your load balancer. Defaults to AWS's default
const ServiceAnnotationLoadBalancerSSLNegotiationPolicy = "service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy"

// ServiceAnnotationLoadBalancerBEProtocol is the annotation used on the service
// to specify the protocol spoken by the backend (pod) behind a listener.
// If `http` (default) or `https`, an HTTPS listener that terminates the
//  connection and parses headers is created.
// If set to `ssl` or `tcp`, a "raw" SSL listener is used.
// If set to `http` and `aws-load-balancer-ssl-cert` is not used then
// a HTTP listener is used.
const ServiceAnnotationLoadBalancerBEProtocol = "service.beta.kubernetes.io/aws-load-balancer-backend-protocol"

// ServiceAnnotationLoadBalancerAdditionalTags is the annotation used on the service
// to specify a comma-separated list of key-value pairs which will be recorded as
// additional tags in the ELB.
// For example: "Key1=Val1,Key2=Val2,KeyNoVal1=,KeyNoVal2"
const ServiceAnnotationLoadBalancerAdditionalTags = "service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags"

// ServiceAnnotationLoadBalancerHCHealthyThreshold is the annotation used on
// the service to specify the number of successive successful health checks
// required for a backend to be considered healthy for traffic.
const ServiceAnnotationLoadBalancerHCHealthyThreshold = "service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold"

// ServiceAnnotationLoadBalancerHCUnhealthyThreshold is the annotation used
// on the service to specify the number of unsuccessful health checks
// required for a backend to be considered unhealthy for traffic
const ServiceAnnotationLoadBalancerHCUnhealthyThreshold = "service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold"

// ServiceAnnotationLoadBalancerHCTimeout is the annotation used on the
// service to specify, in seconds, how long to wait before marking a health
// check as failed.
const ServiceAnnotationLoadBalancerHCTimeout = "service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout"

// ServiceAnnotationLoadBalancerHCInterval is the annotation used on the
// service to specify, in seconds, the interval between health checks.
const ServiceAnnotationLoadBalancerHCInterval = "service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval"

// ServiceAnnotationLoadBalancerEIPAllocations is the annotation used on the
// service to specify a comma separated list of EIP allocations to use as
// static IP addresses for the NLB. Only supported on elbv2 (NLB)
const ServiceAnnotationLoadBalancerEIPAllocations = "service.beta.kubernetes.io/aws-load-balancer-eip-allocations"

// Event key when a volume is stuck on attaching state when being attached to a volume
const volumeAttachmentStuck = "VolumeAttachmentStuck"

// Indicates that a node has volumes stuck in attaching state and hence it is not fit for scheduling more pods
const nodeWithImpairedVolumes = "NodeWithImpairedVolumes"

const (
	// volumeAttachmentConsecutiveErrorLimit is the number of consecutive errors we will ignore when waiting for a volume to attach/detach
	volumeAttachmentStatusConsecutiveErrorLimit = 10
	// most attach/detach operations on AWS finish within 1-4 seconds
	// By using 1 second starting interval with a backoff of 1.8
	// we get -  [1, 1.8, 3.24, 5.832000000000001, 10.4976]
	// in total we wait for 2601 seconds
	volumeAttachmentStatusInitialDelay = 1 * time.Second
	volumeAttachmentStatusFactor       = 1.8
	volumeAttachmentStatusSteps        = 13

	// createTag* is configuration of exponential backoff for CreateTag call. We
	// retry mainly because if we create an object, we cannot tag it until it is
	// "fully created" (eventual consistency). Starting with 1 second, doubling
	// it every step and taking 9 steps results in 255 second total waiting
	// time.
	createTagInitialDelay = 1 * time.Second
	createTagFactor       = 2.0
	createTagSteps        = 9

	// volumeCreate* is configuration of exponential backoff for created volume.
	// On a random AWS account (shared among several developers) it took 4s on
	// average, 8s max.
	volumeCreateInitialDelay  = 5 * time.Second
	volumeCreateBackoffFactor = 1.2
	volumeCreateBackoffSteps  = 10

	// Number of node names that can be added to a filter. The AWS limit is 200
	// but we are using a lower limit on purpose
	filterNodeLimit = 150
)

// awsTagNameMasterRoles is a set of well-known AWS tag names that indicate the instance is a master
// The major consequence is that it is then not considered for AWS zone discovery for dynamic volume creation.
var awsTagNameMasterRoles = sets.NewString("kubernetes.io/role/master", "k8s.io/role/master")

// Maps from backend protocol to ELB protocol
var backendProtocolMapping = map[string]string{
	"https": "https",
	"http":  "https",
	"ssl":   "ssl",
	"tcp":   "ssl",
}

// MaxReadThenCreateRetries sets the maximum number of attempts we will make when
// we read to see if something exists and then try to create it if we didn't find it.
// This can fail once in a consistent system if done in parallel
// In an eventually consistent system, it could fail unboundedly
const MaxReadThenCreateRetries = 30

// DefaultVolumeType specifies which storage to use for newly created Volumes
// TODO: Remove when user/admin can configure volume types and thus we don't
// need hardcoded defaults.
const DefaultVolumeType = "gp2"


// AWS volume types
const (
	// Provisioned IOPS SSD
	VolumeTypeIO1 = "io1"
	// General Purpose SSD
	VolumeTypeGP2 = "gp2"
	// Cold HDD (sc1)
	VolumeTypeSC1 = "sc1"
	// Throughput Optimized HDD
	VolumeTypeST1 = "st1"
)

// AWS provisioning limits.
// Source: http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html
const (
	MinTotalIOPS = 100
	MaxTotalIOPS = 20000
)

// TagNameClusterNode
// logically independent clusters running in the same AZ.
// The tag key = OscK8sNodeName
// The tag value host name kubernetes.io/hostname
const TagNameClusterNode = "OscK8sNodeName"

// TagNameClusterNode
// logically independent clusters running in the same AZ.
// The tag key = OscK8sNodeName
// The tag value host name kubernetes.io/hostname
const TagNameMainSG = "OscK8sMainSG/"

var aliveFilter = []string{
	ec2.InstanceStateNamePending,
	ec2.InstanceStateNameRunning,
	ec2.InstanceStateNameShuttingDown,
	ec2.InstanceStateNameStopping,
	ec2.InstanceStateNameStopped,
}
