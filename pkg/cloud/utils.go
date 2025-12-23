/*
SPDX-FileCopyrightText: 2025 Outscale SAS <opensource@outscale.com>

SPDX-License-Identifier: BSD-3-Clause
*/
package cloud

import (
	"errors"
	"strconv"

	"github.com/outscale/osc-sdk-go/v3/pkg/osc"
	"google.golang.org/grpc/codes"
)

func GRPCCode(err error) codes.Code {
	if errors.Is(err, ErrNotFound) {
		return codes.NotFound
	}
	if errors.Is(err, ErrAlreadyExists) {
		return codes.AlreadyExists
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
	case code >= 10000 && code < 11000: // InsufficientCapacity, TooManyResources (QuotaExceeded)
		return codes.ResourceExhausted // https://github.com/container-storage-interface/spec/blob/master/spec.md#createvolume-errors
	case code == 4116 || code < 4117: // ErrorNextPageTokenExpired, ErrorInvalidNextPageTokenValue
		return codes.Aborted // https://github.com/container-storage-interface/spec/blob/master/spec.md#listsnapshots-errors
	case code == 4202 || code == 4029: // ErrorInvalidIopsSizeRatio, ErrorInvalidIops
		return codes.InvalidArgument // https://github.com/container-storage-interface/spec/blob/master/spec.md#controllermodifyvolume-errors
	default:
		return codes.Internal
	}
}

func isVolumeNotFoundError(err error) bool {
	if apiErr := osc.AsErrorResponse(err); apiErr != nil {
		if apiErr.GetCode() == "5064" {
			return true
		}
	}
	return false
}

func isSnapshotNotFoundError(err error) bool {
	if apiErr := osc.AsErrorResponse(err); apiErr != nil {
		if apiErr.GetCode() == "5054" {
			return true
		}
	}
	return false
}
