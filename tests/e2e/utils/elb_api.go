package e2eutils

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	elbApi "github.com/aws/aws-sdk-go/service/elb"

	osc "github.com/outscale-dev/cloud-provider-osc/cloud-controller-manager/osc"
)

//ElbAPI instanciate elb service
func ElbAPI() (osc.ELB, error) {
	sess, err := osc.NewSession()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize AWS session: %v", err)
	}
	elbClient := elbApi.New(sess)
	return elbClient, nil
}

//RemoveLbInst remove instance from lb
func RemoveLbInst(elb osc.ELB, lbName string, lbInstances []*elbApi.Instance) error {
	fmt.Printf("Instances removed from load-balancer %s", lbName)
	deregisterRequest := &elbApi.DeregisterInstancesFromLoadBalancerInput{}
	deregisterRequest.Instances = lbInstances
	deregisterRequest.LoadBalancerName = aws.String(lbName)
	_, err := elb.DeregisterInstancesFromLoadBalancer(deregisterRequest)
	if err != nil {
		return err
	}
	return nil
}

//GetLb describe an LB
func GetLb(elb osc.ELB, name string) (*elbApi.LoadBalancerDescription, error) {
	request := &elbApi.DescribeLoadBalancersInput{}
	request.LoadBalancerNames = []*string{&name}

	response, err := elb.DescribeLoadBalancers(request)
	if err != nil {
		if awsError, ok := err.(awserr.Error); ok {
			if awsError.Code() == "LoadBalancerNotFound" {
				return nil, nil
			}
		}
		return nil, err
	}

	var ret *elbApi.LoadBalancerDescription
	for _, loadBalancer := range response.LoadBalancerDescriptions {
		if ret != nil {
			return nil, fmt.Errorf("Found multiple load balancers with name: %s", name)
		}
		ret = loadBalancer
	}

	return ret, nil
}
