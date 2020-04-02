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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
)

// ********************* CCM CloudConfig Def & functions *********************

// CloudConfig wraps the settings for the AWS cloud provider.
// NOTE: Cloud config files should follow the same Kubernetes deprecation policy as
// flags or CLIs. Config fields should not change behavior in incompatible ways and
// should be deprecated for at least 2 release prior to removing.
// See https://kubernetes.io/docs/reference/using-api/deprecation-policy/#deprecating-a-flag-or-cli
// for more details.
type CloudConfig struct {
	Global struct {
		// TODO: Is there any use for this?  We can get it from the instance metadata service
		// Maybe if we're not running on AWS, e.g. bootstrap; for now it is not very useful
		Zone string

		// The AWS VPC flag enables the possibility to run the master components
		// on a different aws account, on a different cloud provider or on-premises.
		// If the flag is set also the KubernetesClusterTag must be provided
		VPC string
		// SubnetID enables using a specific subnet to use for ELB's
		SubnetID string
		// RouteTableID enables using a specific RouteTable
		RouteTableID string

		// RoleARN is the IAM role to assume when interaction with AWS APIs.
		RoleARN string

		// KubernetesClusterTag is the legacy cluster id we'll use to identify our cluster resources
		KubernetesClusterTag string
		// KubernetesClusterID is the cluster id we'll use to identify our cluster resources
		KubernetesClusterID string

		//The aws provider creates an inbound rule per load balancer on the node security
		//group. However, this can run into the AWS security group rule limit of 50 if
		//many LoadBalancers are created.
		//
		//This flag disables the automatic ingress creation. It requires that the user
		//has setup a rule that allows inbound traffic on kubelet ports from the
		//local VPC subnet (so load balancers can access it). E.g. 10.82.0.0/16 30000-32000.
		DisableSecurityGroupIngress bool

		//AWS has a hard limit of 500 security groups. For large clusters creating a security group for each ELB
		//can cause the max number of security groups to be reached. If this is set instead of creating a new
		//Security group for each ELB this security group will be used instead.
		ElbSecurityGroup string

		//During the instantiation of an new AWS cloud provider, the detected region
		//is validated against a known set of regions.
		//
		//In a non-standard, AWS like environment (e.g. Eucalyptus), this check may
		//be undesirable.  Setting this to true will disable the check and provide
		//a warning that the check was skipped.  Please note that this is an
		//experimental feature and work-in-progress for the moment.  If you find
		//yourself in an non-AWS cloud and open an issue, please indicate that in the
		//issue body.
		DisableStrictZoneCheck bool
	}
	// [ServiceOverride "1"]
	//  Service = s3
	//  Region = region1
	//  URL = https://s3.foo.bar
	//  SigningRegion = signing_region
	//  SigningMethod = signing_method
	//
	//  [ServiceOverride "2"]
	//     Service = ec2
	//     Region = region2
	//     URL = https://ec2.foo.bar
	//     SigningRegion = signing_region
	//     SigningMethod = signing_method
	ServiceOverride map[string]*struct {
		Service       string
		Region        string
		URL           string
		SigningRegion string
		SigningMethod string
		SigningName   string
	}
}

func (cfg *CloudConfig) validateOverrides() error {
	if len(cfg.ServiceOverride) == 0 {
		return nil
	}
	set := make(map[string]bool)
	for onum, ovrd := range cfg.ServiceOverride {
		// Note: gcfg does not space trim, so we have to when comparing to empty string ""
		name := strings.TrimSpace(ovrd.Service)
		if name == "" {
			return fmt.Errorf("service name is missing [Service is \"\"] in override %s", onum)
		}
		// insure the map service name is space trimmed
		ovrd.Service = name

		region := strings.TrimSpace(ovrd.Region)
		if region == "" {
			return fmt.Errorf("service region is missing [Region is \"\"] in override %s", onum)
		}
		// insure the map region is space trimmed
		ovrd.Region = region

		url := strings.TrimSpace(ovrd.URL)
		if url == "" {
			return fmt.Errorf("url is missing [URL is \"\"] in override %s", onum)
		}
		signingRegion := strings.TrimSpace(ovrd.SigningRegion)
		if signingRegion == "" {
			return fmt.Errorf("signingRegion is missing [SigningRegion is \"\"] in override %s", onum)
		}
		signature := name + "_" + region
		if set[signature] {
			return fmt.Errorf("duplicate entry found for service override [%s] (%s in %s)", onum, name, region)
		}
		set[signature] = true
	}
	return nil
}

func (cfg *CloudConfig) getResolver() endpoints.ResolverFunc {
	defaultResolver := endpoints.DefaultResolver()
	defaultResolverFn := func(service, region string,
		optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		return defaultResolver.EndpointFor(service, region, optFns...)
	}
	if len(cfg.ServiceOverride) == 0 {
		return defaultResolverFn
	}

	return func(service, region string,
		optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		for _, override := range cfg.ServiceOverride {
			if override.Service == service && override.Region == region {
				return endpoints.ResolvedEndpoint{
					URL:           override.URL,
					SigningRegion: override.SigningRegion,
					SigningMethod: override.SigningMethod,
					SigningName:   override.SigningName,
				}, nil
			}
		}
		return defaultResolver.EndpointFor(service, region, optFns...)
	}
}
