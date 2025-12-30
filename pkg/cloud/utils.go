/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"errors"
	"slices"
	"strconv"

	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"google.golang.org/grpc/codes"
)

func GRPCCode(err error) codes.Code {
	if errors.Is(err, ErrNotFound) {
		return codes.NotFound
	}
	apiErr := osc.AsErrorResponse(err)
	if apiErr == nil {
		return codes.Internal
	}
	apiCode := apiErr.GetCode()
	if apiCode == "" {
		return codes.Internal
	}
	code, ierr := strconv.Atoi(apiCode)
	if ierr != nil {
		return codes.Internal
	}
	switch {
	case code == 5064:
		// InvalidResource (The VolumeId doesn't exist.)
		return codes.NotFound
	case code >= 10000 && code < 11000:
		// InsufficientCapacity, TooManyResources (QuotaExceeded)
		return codes.ResourceExhausted // https://github.com/container-storage-interface/spec/blob/master/spec.md#createvolume-errors
	case code == 4116 || code == 4117:
		// ErrorNextPageTokenExpired, ErrorInvalidNextPageTokenValue
		return codes.Aborted // https://github.com/container-storage-interface/spec/blob/master/spec.md#listsnapshots-errors
	case slices.Contains([]int{4019, 4029, 4061, 4078, 4125, 4202, 4203}, code):
		// ErrorInvalidDeviceName, ErrorInvalidIops, ErrorInvalidSnapshotId, ErrorInvalidVolumeSize, TooSmallVolumeSize, ErrorInvalidIopsSizeRatio, InvalidSnapshotSize
		return codes.InvalidArgument // https://github.com/container-storage-interface/spec/blob/master/spec.md#controllermodifyvolume-errors
	default:
		return codes.Internal
	}
}

func hasErrorCode(err error, code string) bool {
	oerr := osc.AsErrorResponse(err)
	if oerr == nil {
		return false
	}
	return oerr.GetCode() == code
}
