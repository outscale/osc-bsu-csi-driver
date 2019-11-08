/*
Copyright 2018 The Kubernetes Authors.

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

package integration

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
//	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/driver"
	"github.com/kubernetes-sigs/aws-ebs-csi-driver/pkg/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
)

const (
	endpoint = "tcp://127.0.0.1:10000"
)

var (
	drv       *driver.Driver
	csiClient *CSIClient
	ec2Client *ec2.EC2
)

func TestIntegration(t *testing.T) {
	flag.Parse()
	RegisterFailHandler(Fail)
	RunSpecs(t, "AWS EBS CSI Driver Integration Tests")
}

var _ = BeforeSuite(func() {
	// Run CSI Driver in its own goroutine
	var err error
	drv, err = driver.NewDriver(driver.WithEndpoint(endpoint))
	Expect(err).To(BeNil())
	go func() {
		err := drv.Run()
		Expect(err).To(BeNil())
	}()

	// Create CSI Controller client
	csiClient, err = newCSIClient()
	Expect(err).To(BeNil(), "Set up Controller Client failed with error")
	Expect(csiClient).NotTo(BeNil())

	// Create EC2 client
	ec2Client, err = newEC2Client()
	Expect(err).To(BeNil(), "Set up EC2 client failed with error")
	Expect(ec2Client).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	drv.Stop()
})

type CSIClient struct {
	ctrl csi.ControllerClient
	node csi.NodeClient
}

func newCSIClient() (*CSIClient, error) {
	opts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) {
				scheme, addr, err := util.ParseEndpoint(endpoint)
				if err != nil {
					return nil, err
				}
				return net.Dial(scheme, addr)
			}),
	}
	grpcClient, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, err
	}

	log.Printf("OSC: return  CSI CLIENT")	
	return &CSIClient{
		ctrl: csi.NewControllerClient(grpcClient),
		node: csi.NewNodeClient(grpcClient),
	}, nil
}

func newMetadata() (cloud.MetadataService, error) {
	myCustomResolver := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
	     return endpoints.ResolvedEndpoint{
	         URL:           "http://169.254.169.254/latest",
	         SigningRegion: "custom-signing-region",
	     }, nil
	}
	s, err := session.NewSession(&aws.Config{
		Region:           aws.String("eu-west-2"),
		EndpointResolver: endpoints.ResolverFunc(myCustomResolver),
	})
	if err != nil {
		return nil, err
	}

	return cloud.NewMetadataService(ec2metadata.New(s))
}

func newEC2Client() (*ec2.EC2, error) {
	m, err := newMetadata()
	if err != nil {
		return nil, err
	}

	provider := []credentials.Provider{
				&credentials.EnvProvider{},
				//&ec2rolecreds.EC2RoleProvider{Client: svc},
				&credentials.SharedCredentialsProvider{},
			}

               myCustomResolver := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		             if service == endpoints.Ec2ServiceID {
	                       return endpoints.ResolvedEndpoint{
	                         URL:           "https://fcu.eu-west-2.outscale.com",
	                         SigningRegion: "eu-west-2",
                                 SigningName: "ec2",
	                     }, nil
                     }
                     return endpoints.DefaultResolver().EndpointFor(service, region, optFns...)
	       }


	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(m.GetRegion()),
		Credentials: credentials.NewChainCredentials(provider),
		CredentialsChainVerboseErrors: aws.Bool(true),
		EndpointResolver: endpoints.ResolverFunc(myCustomResolver),
	}))
	log.Printf("OSC: ec2.new")
	return ec2.New(sess), nil
}

func logf(format string, args ...interface{}) {
	fmt.Fprintln(GinkgoWriter, fmt.Sprintf(format, args...))
}
