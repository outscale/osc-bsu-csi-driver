/*
Copyright 2019 The Kubernetes Authors.

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

package aws

import (
	"runtime"

	"k8s.io/klog"
	"github.com/aws/aws-sdk-go/aws/endpoints"
)



func OscSetupMetadataResolver() (endpoints.ResolverFunc) {
    return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
        return endpoints.ResolvedEndpoint{
            URL:           "http://169.254.169.254/latest",
            SigningRegion: "custom-signing-region",
        }, nil
    }
}

func OscEndpoint(region string, service string) (string) {
    return "https://" + service + "." + region + ".outscale.com"
}

func OscSetupServiceResolver(region string) (endpoints.ResolverFunc) {

    return func(service, region string, optFns ...func(*endpoints.Options))(endpoints.ResolvedEndpoint, error) {

        supported_service := map[string]string  {
            endpoints.Ec2ServiceID:                    "fcu",
            endpoints.ElasticloadbalancingServiceID:   "lbu",
            endpoints.IamServiceID:                    "eim",
            endpoints.DirectconnectServiceID:          "directlink",
            endpoints.KmsServiceID:                    "kms",
        }
        var osc_service string
        var ok bool
        if osc_service, ok =  supported_service[service]; ok {
            return endpoints.ResolvedEndpoint{
                    URL:           OscEndpoint(region, osc_service),
                    SigningRegion: region,
                    SigningName:   service,
            }, nil
        } else {
            return endpoints.DefaultResolver().EndpointFor(service, region, optFns...)
        }
    }
}


func debugGetFrame(skipFrames int) runtime.Frame {
	// We need the frame at index skipFrames+2, since we never want runtime.Callers and getFrame
	targetFrameIndex := skipFrames + 2

	// Set size to targetFrameIndex+2 to ensure we have room for one more caller than we need
	programCounters := make([]uintptr, targetFrameIndex+2)
	n := runtime.Callers(0, programCounters)

	frame := runtime.Frame{Function: "unknown"}
	if n > 0 {
		frames := runtime.CallersFrames(programCounters[:n])
		for more, frameIndex := true, 0; more && frameIndex <= targetFrameIndex; frameIndex++ {
			var frameCandidate runtime.Frame
			frameCandidate, more = frames.Next()
			if frameIndex == targetFrameIndex {
				frame = frameCandidate
			}
		}
	}

	return frame
}

func debugPrintCallerFunctionName() {
	called := debugGetFrame(1)
	caller := debugGetFrame(2)
	klog.V(10).Infof("Debug Stack => %s(%s:%d) called by %s(%s:%d)",
	 			called.Function, called.File, called.Line,
	 			caller.Function, caller.File, caller.Line )
}

func debugGetCurrentFunctionName() string {
	// Skip debugGetCurrentFunctionName
	return debugGetFrame(1).Function
}

func debugGetCallerFunctionName() string {
	// Skip debugGetCallerFunctionName and the function to get the caller of
	return debugGetFrame(2).Function
}
