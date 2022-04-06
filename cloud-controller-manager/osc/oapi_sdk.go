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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// ********************* CCM oscSdkCompute Def & functions *********************

// oscSdkCompute is an implementation of the EC2 interface, backed by aws-sdk-go
type oscSdkCompute struct {
	oapi *ec2.EC2
}

// Implementation of EC2.Instances
func (s *oscSdkCompute) ReadVms(request *ec2.DescribeInstancesInput) ([]*ec2.Instance, error) {
	// Instances are paged
	results := []*ec2.Instance{}
	var nextToken *string
	requestTime := time.Now()
	for {
		response, err := s.oapi.DescribeInstances(request)
		if err != nil {
			recordAWSMetric("describe_instance", 0, err)
			return nil, fmt.Errorf("error listing AWS instances: %q", err)
		}

		for _, reservation := range response.Reservations {
			results = append(results, reservation.Instances...)
		}

		nextToken = response.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
		request.NextToken = nextToken
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_instance", timeTaken, nil)
	return results, nil
}

// Implements EC2.ReadSecurityGroups
func (s *oscSdkCompute) ReadSecurityGroups(request *ec2.DescribeSecurityGroupsInput) ([]*ec2.SecurityGroup, error) {
	// Security groups are paged
	results := []*ec2.SecurityGroup{}
	var nextToken *string
	requestTime := time.Now()
	for {
		response, err := s.oapi.DescribeSecurityGroups(request)
		if err != nil {
			recordAWSMetric("describe_security_groups", 0, err)
			return nil, fmt.Errorf("error listing AWS security groups: %q", err)
		}

		results = append(results, response.SecurityGroups...)

		nextToken = response.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
		request.NextToken = nextToken
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_security_groups", timeTaken, nil)
	return results, nil
}

func (s *oscSdkCompute) DescribeSubnets(request *ec2.DescribeSubnetsInput) ([]*ec2.Subnet, error) {
	// Subnets are not paged
	response, err := s.oapi.DescribeSubnets(request)
	if err != nil {
		return nil, fmt.Errorf("error listing AWS subnets: %q", err)
	}
	return response.Subnets, nil
}

func (s *oscSdkCompute) CreateSecurityGroup(request *ec2.CreateSecurityGroupInput) (*ec2.CreateSecurityGroupOutput, error) {
	return s.oapi.CreateSecurityGroup(request)
}

func (s *oscSdkCompute) DeleteSecurityGroup(request *ec2.DeleteSecurityGroupInput) (*ec2.DeleteSecurityGroupOutput, error) {
	return s.oapi.DeleteSecurityGroup(request)
}

func (s *oscSdkCompute) CreateSecurityGroupRule(request *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return s.oapi.AuthorizeSecurityGroupIngress(request)
}

func (s *oscSdkCompute) DeleteSecurityGroupRule(request *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return s.oapi.RevokeSecurityGroupIngress(request)
}

func (s *oscSdkCompute) CreateTags(request *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	debugPrintCallerFunctionName()
	requestTime := time.Now()
	resp, err := s.oapi.CreateTags(request)
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("create_tags", timeTaken, err)
	return resp, err
}

func (s *oscSdkCompute) ReadRouteTables(request *ec2.DescribeRouteTablesInput) ([]*ec2.RouteTable, error) {
	results := []*ec2.RouteTable{}
	var nextToken *string
	requestTime := time.Now()
	for {
		response, err := s.oapi.DescribeRouteTables(request)
		if err != nil {
			recordAWSMetric("describe_route_tables", 0, err)
			return nil, fmt.Errorf("error listing AWS route tables: %q", err)
		}

		results = append(results, response.RouteTables...)

		nextToken = response.NextToken
		if aws.StringValue(nextToken) == "" {
			break
		}
		request.NextToken = nextToken
	}
	timeTaken := time.Since(requestTime).Seconds()
	recordAWSMetric("describe_route_tables", timeTaken, nil)
	return results, nil
}

func (s *oscSdkCompute) CreateRoute(request *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	return s.oapi.CreateRoute(request)
}

func (s *oscSdkCompute) DeleteRoute(request *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	return s.oapi.DeleteRoute(request)
}

func (s *oscSdkCompute) UpdateVm(request *ec2.ModifyInstanceAttributeInput) (*ec2.ModifyInstanceAttributeOutput, error) {
	return s.oapi.ModifyInstanceAttribute(request)
}

func (s *oscSdkCompute) ReadNets(request *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	return s.oapi.DescribeVpcs(request)
}
