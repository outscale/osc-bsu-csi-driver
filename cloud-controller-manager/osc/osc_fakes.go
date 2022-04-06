//go:build !providerless
// +build !providerless

/*
Copyright 2017 The Kubernetes Authors.

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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"k8s.io/klog/v2"
)

// FakeAWSServices is an fake AWS session used for testing
type FakeOscServices struct {
	region                      string
	instances                   []*ec2.Instance
	selfInstance                *ec2.Instance
	networkInterfacesMacs       []string
	networkInterfacesPrivateIPs [][]string
	networkInterfacesVpcIDs     []string

	ec2      FakeCompute
	elb      ELB
	metadata EC2Metadata
}

// NewFakeAWSServices creates a new FakeAWSServices
func NewFakeAWSServices(clusterID string) *FakeOscServices {
	s := &FakeOscServices{}
	s.region = "us-east-1"
	s.ec2 = &FakeComputeImpl{aws: s}
	s.elb = &FakeELB{aws: s}
	s.metadata = &FakeMetadata{aws: s}

	s.networkInterfacesMacs = []string{"aa:bb:cc:dd:ee:00", "aa:bb:cc:dd:ee:01"}
	s.networkInterfacesVpcIDs = []string{"vpc-mac0", "vpc-mac1"}

	selfInstance := &ec2.Instance{}
	selfInstance.InstanceId = aws.String("i-self")
	selfInstance.Placement = &ec2.Placement{
		AvailabilityZone: aws.String("us-east-1a"),
	}
	selfInstance.PrivateDnsName = aws.String("ip-172-20-0-100.ec2.internal")
	selfInstance.PrivateIpAddress = aws.String("192.168.0.1")
	selfInstance.PublicIpAddress = aws.String("1.2.3.4")
	s.selfInstance = selfInstance
	s.instances = []*ec2.Instance{selfInstance}

	var tag ec2.Tag
	tag.Key = aws.String(TagNameKubernetesClusterLegacy)
	tag.Value = aws.String(clusterID)
	selfInstance.Tags = []*ec2.Tag{&tag}

	return s
}

// WithAz sets the ec2 placement availability zone
func (s *FakeOscServices) WithAz(az string) *FakeOscServices {
	if s.selfInstance.Placement == nil {
		s.selfInstance.Placement = &ec2.Placement{}
	}
	s.selfInstance.Placement.AvailabilityZone = aws.String(az)
	return s
}

// Compute returns a fake EC2 client
func (s *FakeOscServices) Compute(region string) (Compute, error) {
	return s.ec2, nil
}

// LoadBalancing returns a fake ELB client
func (s *FakeOscServices) LoadBalancing(region string) (ELB, error) {
	return s.elb, nil
}

// Metadata returns a fake EC2Metadata client
func (s *FakeOscServices) Metadata() (EC2Metadata, error) {
	return s.metadata, nil
}

// FakeCompute is a fake Compute client used for testing
type FakeCompute interface {
	Compute
	CreateSubnet(*ec2.Subnet) (*ec2.CreateSubnetOutput, error)
	RemoveSubnets()
	CreateRouteTable(*ec2.RouteTable) (*ec2.CreateRouteTableOutput, error)
	RemoveRouteTables()
}

// FakeEC2Impl is an implementation of the FakeEC2 interface used for testing
type FakeComputeImpl struct {
	aws                      *FakeOscServices
	Subnets                  []*ec2.Subnet
	DescribeSubnetsInput     *ec2.DescribeSubnetsInput
	RouteTables              []*ec2.RouteTable
	DescribeRouteTablesInput *ec2.DescribeRouteTablesInput
}

// DescribeVms returns fake instance descriptions
func (ec2i *FakeComputeImpl) ReadVms(request *ec2.DescribeInstancesInput) ([]*ec2.Instance, error) {
	matches := []*ec2.Instance{}
	for _, instance := range ec2i.aws.instances {
		if request.InstanceIds != nil {
			if instance.InstanceId == nil {
				klog.Warning("Instance with no instance id: ", instance)
				continue
			}

			found := false
			for _, instanceID := range request.InstanceIds {
				if *instanceID == *instance.InstanceId {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if request.Filters != nil {
			allMatch := true
			for _, filter := range request.Filters {
				if !instanceMatchesFilter(instance, filter) {
					allMatch = false
					break
				}
			}
			if !allMatch {
				continue
			}
		}
		matches = append(matches, instance)
	}

	return matches, nil
}

// ReadSecurityGroups is not implemented but is required for interface
// conformance
func (ec2i *FakeComputeImpl) ReadSecurityGroups(request *ec2.DescribeSecurityGroupsInput) ([]*ec2.SecurityGroup, error) {
	panic("Not implemented")
}

// CreateSecurityGroup is not implemented but is required for interface
// conformance
func (ec2i *FakeComputeImpl) CreateSecurityGroup(*ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	panic("Not implemented")
}

// DeleteSecurityGroup is not implemented but is required for interface
// conformance
func (ec2i *FakeComputeImpl) DeleteSecurityGroup(*ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	panic("Not implemented")
}

// CreateSecurityGroupRule is not implemented but is required for
// interface conformance
func (ec2i *FakeComputeImpl) CreateSecurityGroupRule(*ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	panic("Not implemented")
}

// DeleteSecurityGroupRule is not implemented but is required for interface
// conformance
func (ec2i *FakeComputeImpl) DeleteSecurityGroupRule(*ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	panic("Not implemented")
}

// CreateSubnet creates fake subnets
func (ec2i *FakeComputeImpl) CreateSubnet(request *ec2.Subnet) (*ec2.CreateSubnetOutput, error) {
	ec2i.Subnets = append(ec2i.Subnets, request)
	response := &ec2.CreateSubnetOutput{
		Subnet: request,
	}
	return response, nil
}

// DescribeSubnets returns fake subnet descriptions
func (ec2i *FakeComputeImpl) DescribeSubnets(request *ec2.DescribeSubnetsInput) ([]*ec2.Subnet, error) {
	ec2i.DescribeSubnetsInput = request
	return ec2i.Subnets, nil
}

// RemoveSubnets clears subnets on client
func (ec2i *FakeComputeImpl) RemoveSubnets() {
	ec2i.Subnets = ec2i.Subnets[:0]
}

// CreateTags is not implemented but is required for interface conformance
func (ec2i *FakeComputeImpl) CreateTags(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	panic("Not implemented")
}

// DescribeRouteTables returns fake route table descriptions
func (ec2i *FakeComputeImpl) DescribeRouteTables(request *ec2.DescribeRouteTablesInput) ([]*ec2.RouteTable, error) {
	ec2i.DescribeRouteTablesInput = request
	return ec2i.RouteTables, nil
}

// CreateRouteTable creates fake route tables
func (ec2i *FakeComputeImpl) CreateRouteTable(request *ec2.RouteTable) (*ec2.CreateRouteTableOutput, error) {
	ec2i.RouteTables = append(ec2i.RouteTables, request)
	response := &ec2.CreateRouteTableOutput{
		RouteTable: request,
	}
	return response, nil
}

// RemoveRouteTables clears route tables on client
func (ec2i *FakeComputeImpl) RemoveRouteTables() {
	ec2i.RouteTables = ec2i.RouteTables[:0]
}

// CreateRoute is not implemented but is required for interface conformance
func (ec2i *FakeComputeImpl) CreateRoute(request *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	panic("Not implemented")
}

// DeleteRoute is not implemented but is required for interface conformance
func (ec2i *FakeComputeImpl) DeleteRoute(request *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	panic("Not implemented")
}

// ModifyInstanceAttribute is not implemented but is required for interface
// conformance
func (ec2i *FakeComputeImpl) UpdateVm(request *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error) {
	panic("Not implemented")
}

// ReadNets returns fake VPC descriptions
func (ec2i *FakeComputeImpl) ReadNets(request *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{Vpcs: []*ec2.Vpc{{CidrBlock: aws.String("172.20.0.0/16")}}}, nil
}

// FakeMetadata is a fake EC2 metadata service client used for testing
type FakeMetadata struct {
	aws *FakeOscServices
}

//GetInstanceID is a fake metadata for testing
func (m *FakeMetadata) GetInstanceID() string {
	return ""
}

//GetInstanceType is a fake metadata for testing
func (m *FakeMetadata) GetInstanceType() string {
	return ""
}

//GetRegion is a fake metadata for testing
func (m *FakeMetadata) GetRegion() string {
	return ""
}

//GetAvailabilityZone is a fake metadata for testing
func (m *FakeMetadata) GetAvailabilityZone() string {
	return ""
}

// GetInstanceIdentityDocument mocks base method
func (m *FakeMetadata) GetInstanceIdentityDocument() (ec2metadata.EC2InstanceIdentityDocument, error) {
	return ec2metadata.EC2InstanceIdentityDocument{}, nil
}

// Available mocks base method
func (m *FakeMetadata) Available() bool {
	return true
}

// GetMetadata returns fake EC2 metadata for testing
func (m *FakeMetadata) GetMetadata(key string) (string, error) {
	networkInterfacesPrefix := "network/interfaces/macs/"
	i := m.aws.selfInstance
	if key == "placement/availability-zone" {
		az := ""
		if i.Placement != nil {
			az = aws.StringValue(i.Placement.AvailabilityZone)
		}
		return az, nil
	} else if key == "instance-id" {
		return aws.StringValue(i.InstanceId), nil
	} else if key == "local-hostname" {
		return aws.StringValue(i.PrivateDnsName), nil
	} else if key == "public-hostname" {
		return aws.StringValue(i.PublicDnsName), nil
	} else if key == "local-ipv4" {
		return aws.StringValue(i.PrivateIpAddress), nil
	} else if key == "public-ipv4" {
		return aws.StringValue(i.PublicIpAddress), nil
	} else if strings.HasPrefix(key, networkInterfacesPrefix) {
		if key == networkInterfacesPrefix {
			return strings.Join(m.aws.networkInterfacesMacs, "/\n") + "/\n", nil
		}

		keySplit := strings.Split(key, "/")
		macParam := keySplit[3]
		if len(keySplit) == 5 && keySplit[4] == "vpc-id" {
			for i, macElem := range m.aws.networkInterfacesMacs {
				if macParam == macElem {
					return m.aws.networkInterfacesVpcIDs[i], nil
				}
			}
		}
		if len(keySplit) == 5 && keySplit[4] == "local-ipv4s" {
			for i, macElem := range m.aws.networkInterfacesMacs {
				if macParam == macElem {
					return strings.Join(m.aws.networkInterfacesPrivateIPs[i], "/\n"), nil
				}
			}
		}

		return "", nil
	}

	return "", nil
}

// FakeELB is a fake ELB client used for testing
type FakeELB struct {
	aws *FakeOscServices
}

// CreateLoadBalancer is not implemented but is required for interface
// conformance
func (elb *FakeELB) CreateLoadBalancer(*elb.CreateLoadBalancerInput) (*elb.CreateLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DeleteLoadBalancer is not implemented but is required for interface
// conformance
func (elb *FakeELB) DeleteLoadBalancer(input *elb.DeleteLoadBalancerInput) (*elb.DeleteLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DescribeLoadBalancers is not implemented but is required for interface
// conformance
func (elb *FakeELB) DescribeLoadBalancers(input *elb.DescribeLoadBalancersInput) (*elb.DescribeLoadBalancersOutput, error) {
	panic("Not implemented")
}

// AddTags is not implemented but is required for interface conformance
func (elb *FakeELB) AddTags(input *elb.AddTagsInput) (*elb.AddTagsOutput, error) {
	panic("Not implemented")
}

// RegisterInstancesWithLoadBalancer is not implemented but is required for
// interface conformance
func (elb *FakeELB) RegisterInstancesWithLoadBalancer(*elb.RegisterInstancesWithLoadBalancerInput) (*elb.RegisterInstancesWithLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DeregisterInstancesFromLoadBalancer is not implemented but is required for
// interface conformance
func (elb *FakeELB) DeregisterInstancesFromLoadBalancer(*elb.DeregisterInstancesFromLoadBalancerInput) (*elb.DeregisterInstancesFromLoadBalancerOutput, error) {
	panic("Not implemented")
}

// DetachLoadBalancerFromSubnets is not implemented but is required for
// interface conformance
func (elb *FakeELB) DetachLoadBalancerFromSubnets(*elb.DetachLoadBalancerFromSubnetsInput) (*elb.DetachLoadBalancerFromSubnetsOutput, error) {
	panic("Not implemented")
}

// AttachLoadBalancerToSubnets is not implemented but is required for interface
// conformance
func (elb *FakeELB) AttachLoadBalancerToSubnets(*elb.AttachLoadBalancerToSubnetsInput) (*elb.AttachLoadBalancerToSubnetsOutput, error) {
	panic("Not implemented")
}

// CreateLoadBalancerListeners is not implemented but is required for interface
// conformance
func (elb *FakeELB) CreateLoadBalancerListeners(*elb.CreateLoadBalancerListenersInput) (*elb.CreateLoadBalancerListenersOutput, error) {
	panic("Not implemented")
}

// DeleteLoadBalancerListeners is not implemented but is required for interface
// conformance
func (elb *FakeELB) DeleteLoadBalancerListeners(*elb.DeleteLoadBalancerListenersInput) (*elb.DeleteLoadBalancerListenersOutput, error) {
	panic("Not implemented")
}

// ApplySecurityGroupsToLoadBalancer is not implemented but is required for
// interface conformance
func (elb *FakeELB) ApplySecurityGroupsToLoadBalancer(*elb.ApplySecurityGroupsToLoadBalancerInput) (*elb.ApplySecurityGroupsToLoadBalancerOutput, error) {
	panic("Not implemented")
}

// ConfigureHealthCheck is not implemented but is required for interface
// conformance
func (elb *FakeELB) ConfigureHealthCheck(*elb.ConfigureHealthCheckInput) (*elb.ConfigureHealthCheckOutput, error) {
	panic("Not implemented")
}

// CreateLoadBalancerPolicy is not implemented but is required for interface
// conformance
func (elb *FakeELB) CreateLoadBalancerPolicy(*elb.CreateLoadBalancerPolicyInput) (*elb.CreateLoadBalancerPolicyOutput, error) {
	panic("Not implemented")
}

// SetLoadBalancerPoliciesForBackendServer is not implemented but is required
// for interface conformance
func (elb *FakeELB) SetLoadBalancerPoliciesForBackendServer(*elb.SetLoadBalancerPoliciesForBackendServerInput) (*elb.SetLoadBalancerPoliciesForBackendServerOutput, error) {
	panic("Not implemented")
}

// SetLoadBalancerPoliciesOfListener is not implemented but is required for
// interface conformance
func (elb *FakeELB) SetLoadBalancerPoliciesOfListener(input *elb.SetLoadBalancerPoliciesOfListenerInput) (*elb.SetLoadBalancerPoliciesOfListenerOutput, error) {
	panic("Not implemented")
}

// DescribeLoadBalancerPolicies is not implemented but is required for
// interface conformance
func (elb *FakeELB) DescribeLoadBalancerPolicies(input *elb.DescribeLoadBalancerPoliciesInput) (*elb.DescribeLoadBalancerPoliciesOutput, error) {
	panic("Not implemented")
}

// DescribeLoadBalancerAttributes is not implemented but is required for
// interface conformance
func (elb *FakeELB) DescribeLoadBalancerAttributes(*elb.DescribeLoadBalancerAttributesInput) (*elb.DescribeLoadBalancerAttributesOutput, error) {
	panic("Not implemented")
}

// ModifyLoadBalancerAttributes is not implemented but is required for
// interface conformance
func (elb *FakeELB) ModifyLoadBalancerAttributes(*elb.ModifyLoadBalancerAttributesInput) (*elb.ModifyLoadBalancerAttributesOutput, error) {
	panic("Not implemented")
}

// expectDescribeLoadBalancers is not implemented but is required for interface
// conformance
func (elb *FakeELB) expectDescribeLoadBalancers(loadBalancerName string) {
	panic("Not implemented")
}

func instanceMatchesFilter(instance *ec2.Instance, filter *ec2.Filter) bool {
	name := *filter.Name
	if name == "private-dns-name" {
		if instance.PrivateDnsName == nil {
			return false
		}
		return contains(filter.Values, *instance.PrivateDnsName)
	}

	if name == "instance-state-name" {
		return contains(filter.Values, *instance.State.Name)
	}

	if name == "tag-key" {
		for _, instanceTag := range instance.Tags {
			if contains(filter.Values, aws.StringValue(instanceTag.Key)) {
				return true
			}
		}
		return false
	}

	if strings.HasPrefix(name, "tag:") {
		tagName := name[4:]
		for _, instanceTag := range instance.Tags {
			if aws.StringValue(instanceTag.Key) == tagName && contains(filter.Values, aws.StringValue(instanceTag.Value)) {
				return true
			}
		}
		return false
	}

	panic("Unknown filter name: " + name)
}

func contains(haystack []*string, needle string) bool {
	for _, s := range haystack {
		// (deliberately panic if s == nil)
		if needle == *s {
			return true
		}
	}
	return false
}
