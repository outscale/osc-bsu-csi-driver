// +build !providerless

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
	"io"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"gopkg.in/gcfg.v1"

	"k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

// ********************* CCM Object Init *********************

var _ cloudprovider.Interface = (*Cloud)(nil)
var _ cloudprovider.Instances = (*Cloud)(nil)
var _ cloudprovider.LoadBalancer = (*Cloud)(nil)
var _ cloudprovider.Routes = (*Cloud)(nil)
var _ cloudprovider.Zones = (*Cloud)(nil)

// ********************* CCM entry point function *********************

// readAWSCloudConfig reads an instance of AWSCloudConfig from config reader.
func readAWSCloudConfig(config io.Reader) (*CloudConfig, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("readAWSCloudConfig(%v)", config)
	var cfg CloudConfig
	var err error

	if config != nil {
		err = gcfg.ReadInto(&cfg, config)
		if err != nil {
			return nil, err
		}
	}

	return &cfg, nil
}

// newAWSCloud creates a new instance of AWSCloud.
// AWSProvider and instanceId are primarily for tests
func newAWSCloud(cfg CloudConfig, awsServices Services) (*Cloud, error) {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("newAWSCloud(%v, %v)", cfg, awsServices)
	// We have some state in the Cloud object - in particular the attaching map
	// Log so that if we are building multiple Cloud objects, it is obvious!
	klog.Infof("Starting OSC cloud provider")

	metadata, err := awsServices.Metadata()
	if err != nil {
		return nil, fmt.Errorf("error creating OSC metadata client: %q", err)
	}

	err = updateConfigZone(&cfg, metadata)
	if err != nil {
		return nil, fmt.Errorf("unable to determine OSC zone from cloud provider config or EC2 instance metadata: %v", err)
	}
	zone := cfg.Global.Zone
	if len(zone) <= 1 {
		return nil, fmt.Errorf("invalid OSC zone in config file: %s", zone)
	}
	regionName, err := azToRegion(zone)
	if err != nil {
		return nil, err
	}

	instances, err := newInstancesV2(zone, metadata)
	if err != nil {
		return nil, err
	}

	if !cfg.Global.DisableStrictZoneCheck {
		if !isRegionValid(regionName, metadata) {
			return nil, fmt.Errorf("not a valid AWS zone (unknown region): %s", zone)
		}
	} else {
		klog.Warningf("Strict OSC zone checking is disabled.  Proceeding with zone: %s", zone)
	}

	klog.Infof("OSC CCM cfg.Global: %v", cfg.Global)
	klog.Infof("OSC CCM cfg: %v", cfg)

	klog.Infof("Init Services/Compute")
	ec2, err := awsServices.Compute(regionName)
	if err != nil {
		return nil, fmt.Errorf("error creating OSC EC2 client: %v", err)
	}
	klog.Infof("Init Services/LoadBalancing")
	elb, err := awsServices.LoadBalancing(regionName)
	if err != nil {
		return nil, fmt.Errorf("error creating OSC ELB client: %v", err)
	}

	awsCloud := &Cloud{
		ec2:       ec2,
		elb:       elb,
		metadata:  metadata,
		cfg:       &cfg,
		region:    regionName,
		instances: instances,
	}
	awsCloud.instanceCache.cloud = awsCloud

	tagged := cfg.Global.KubernetesClusterTag != "" || cfg.Global.KubernetesClusterID != ""

	if cfg.Global.VPC != "" && (cfg.Global.SubnetID != "" || cfg.Global.RoleARN != "") && tagged {
		// When the master is running on a different AWS account, cloud provider or on-premise
		// build up a dummy instance and use the VPC from the nodes account
		klog.Info("Master is configured to run on a different AWS account, different cloud provider or on-premises")
		awsCloud.selfAWSInstance = &awsInstance{
			nodeName: "master-dummy",
			vpcID:    cfg.Global.VPC,
			subnetID: cfg.Global.SubnetID,
		}
		awsCloud.vpcID = cfg.Global.VPC
	} else {
		selfAWSInstance, err := awsCloud.buildSelfAWSInstance()
		if err != nil {
			return nil, err
		}
		awsCloud.selfAWSInstance = selfAWSInstance
		awsCloud.vpcID = selfAWSInstance.vpcID
		klog.Infof("OSC CCM Instance (%v)", selfAWSInstance)
		klog.Infof("OSC CCM vpcID (%v)", selfAWSInstance.vpcID)

	}

	if cfg.Global.KubernetesClusterTag != "" || cfg.Global.KubernetesClusterID != "" {
		if err := awsCloud.tagging.init(cfg.Global.KubernetesClusterTag, cfg.Global.KubernetesClusterID); err != nil {
			return nil, err
		}
	} else {
		// TODO: Clean up double-API query
		info, err := awsCloud.selfAWSInstance.describeInstance()
		if err != nil {
			return nil, err
		}
		if err := awsCloud.tagging.initFromTags(info.Tags); err != nil {
			return nil, err
		}
	}
	klog.Infof("OSC CCM awsCloud %v", awsCloud)
	return awsCloud, nil
}

func init() {
	debugPrintCallerFunctionName()
	klog.V(10).Infof("init()")
	registerMetrics()
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		cfg, err := readAWSCloudConfig(config)
		if err != nil {
			return nil, fmt.Errorf("unable to read OSC cloud provider config file: %v", err)
		}

		if err = cfg.validateOverrides(); err != nil {
			return nil, fmt.Errorf("unable to validate custom endpoint overrides: %v", err)
		}

		provider := []credentials.Provider{
			&credentials.EnvProvider{},
			&credentials.SharedCredentialsProvider{},
		}

		creds := credentials.NewChainCredentials(provider)

		aws := newAWSSDKProvider(creds, cfg)
		return newAWSCloud(*cfg, aws)
	})
}
