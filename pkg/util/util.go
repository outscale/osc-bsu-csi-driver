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

package util

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
)

const (
	GiB int64 = 1024 * 1024 * 1024
)

// RoundUpBytes rounds up the volume size in bytes upto multiplications of GiB
// in the unit of Bytes
func RoundUpBytes(volumeSizeBytes int64) int64 {
	return roundUpSize(volumeSizeBytes, GiB) * GiB
}

// RoundUpGiB rounds up the volume size in bytes upto multiplications of GiB
// in the unit of GiB
func RoundUpGiB(volumeSizeBytes int64) int32 {
	return int32(roundUpSize(volumeSizeBytes, GiB))
}

// BytesToGiB converts Bytes to GiB
func BytesToGiB(volumeSizeBytes int64) int32 {
	return int32(volumeSizeBytes / GiB)
}

// GiBToBytes converts GiB to Bytes
func GiBToBytes(volumeSizeGiB int32) int64 {
	return int64(volumeSizeGiB) * GiB
}

func ParseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", fmt.Errorf("could not parse endpoint: %w", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "tcp":
	case "unix":
		addr = path.Join("/", addr)
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return "", "", fmt.Errorf("could not remove unix domain socket %q: %w", addr, err)
		}
	default:
		return "", "", fmt.Errorf("unsupported protocol: %s", scheme)
	}

	return scheme, addr, nil
}

// TODO: check division by zero and int overflow
func roundUpSize(volumeSizeBytes int64, allocationUnitBytes int64) int64 {
	return (volumeSizeBytes + allocationUnitBytes - 1) / allocationUnitBytes
}

func OscSetupMetadataResolver() endpoints.ResolverFunc {
	return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		return endpoints.ResolvedEndpoint{
			URL:           "http://169.254.169.254/latest",
			SigningRegion: "custom-signing-region",
		}, nil
	}
}

func OscEndpoint(region string, service string) string {
	return "https://" + service + "." + region + ".outscale.com"
}

func OscSetupServiceResolver(region string) endpoints.ResolverFunc {
	return func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		supported_service := map[string]string{
			endpoints.Ec2ServiceID:                  "fcu",
			endpoints.ElasticloadbalancingServiceID: "lbu",
			endpoints.IamServiceID:                  "eim",
			endpoints.DirectconnectServiceID:        "directlink",
		}
		var osc_service string
		var ok bool
		if osc_service, ok = supported_service[service]; ok {
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

func Getenv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
