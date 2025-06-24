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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundUpBytes(t *testing.T) {
	var sizeInBytes int64 = 1024
	actual := RoundUpBytes(sizeInBytes)
	assert.Equal(t, GiB, actual)
}

func TestRoundUpGiB(t *testing.T) {
	actual := RoundUpGiB(1)
	assert.Equal(t, int32(1), actual)
}

func TestBytesToGiB(t *testing.T) {
	actual := BytesToGiB(5 * GiB)
	assert.Equal(t, int32(5), actual)
}

func TestGiBToBytes(t *testing.T) {
	actual := GiBToBytes(3)
	assert.Equal(t, 3*GiB, actual)
}

func TestParseEndpoint(t *testing.T) {
	testCases := []struct {
		name      string
		endpoint  string
		expScheme string
		expAddr   string
		expErr    error
	}{
		{
			name:      "valid unix endpoint 1",
			endpoint:  "unix:///csi/csi.sock",
			expScheme: "unix",
			expAddr:   "/csi/csi.sock",
		},
		{
			name:      "valid unix endpoint 2",
			endpoint:  "unix://csi/csi.sock",
			expScheme: "unix",
			expAddr:   "/csi/csi.sock",
		},
		{
			name:      "valid unix endpoint 3",
			endpoint:  "unix:/csi/csi.sock",
			expScheme: "unix",
			expAddr:   "/csi/csi.sock",
		},
		{
			name:      "valid tcp endpoint",
			endpoint:  "tcp:///127.0.0.1/",
			expScheme: "tcp",
			expAddr:   "/127.0.0.1",
		},
		{
			name:      "valid tcp endpoint",
			endpoint:  "tcp:///127.0.0.1",
			expScheme: "tcp",
			expAddr:   "/127.0.0.1",
		},
		{
			name:     "invalid endpoint",
			endpoint: "http://127.0.0.1",
			expErr:   errors.New("unsupported protocol: http"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scheme, addr, err := ParseEndpoint(tc.endpoint)

			if tc.expErr != nil {
				require.EqualError(t, err, tc.expErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expScheme, scheme)
				assert.Equal(t, tc.expAddr, addr)
			}
		})
	}
}
