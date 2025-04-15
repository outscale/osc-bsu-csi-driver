package cloud

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	osc "github.com/outscale/osc-sdk-go/v2"
	"google.golang.org/grpc/codes"
)

type GRPCCodeEr interface {
	GRPCCode() codes.Code
}

type OAPIError struct {
	errors []osc.Errors
}

func (err OAPIError) Error() string {
	if len(err.errors) == 0 {
		return "unknown error"
	}
	oe := err.errors[0]
	str := oe.GetCode() + "/" + oe.GetType()
	details := oe.GetDetails()
	if details != "" {
		str += " (" + details + ")"
	}
	return str
}

func (err OAPIError) GRPCCode() codes.Code {
	if len(err.errors) == 0 {
		return codes.Internal
	}
	code, ierr := strconv.Atoi(err.errors[0].GetCode())
	if ierr != nil {
		return codes.Internal
	}
	switch {
	case code >= 10000 && code < 11000:
		return codes.ResourceExhausted
	default:
		return codes.Internal
	}
}

var _ GRPCCodeEr = OAPIError{}

func GRPCCode(err error) codes.Code {
	if grpcErr, ok := err.(GRPCCodeEr); ok {
		return grpcErr.GRPCCode()
	}
	if errors.Is(err, ErrNotFound) {
		return codes.NotFound
	}
	if errors.Is(err, ErrAlreadyExists) {
		return codes.AlreadyExists
	}

	return codes.Internal
}

func extractOAPIError(err error, httpRes *http.Response) error {
	var genericError osc.GenericOpenAPIError
	if errors.As(err, &genericError) {
		errorsResponse, ok := genericError.Model().(osc.ErrorResponse)
		if ok && len(*errorsResponse.Errors) > 0 {
			return OAPIError{errors: *errorsResponse.Errors}
		}
	}
	if httpRes != nil {
		return fmt.Errorf("http error %w", err)
	}
	return err
}

func extractErrors(err error) (*osc.Errors, bool) {
	var (
		errs         []osc.Errors
		genericError osc.GenericOpenAPIError
		oapiError    OAPIError
	)
	switch {
	case errors.As(err, &genericError):
		errorsResponse, ok := genericError.Model().(osc.ErrorResponse)
		if ok {
			errs = errorsResponse.GetErrors()
		}
	case errors.As(err, &oapiError):
		errs = oapiError.errors
	}
	if len(errs) > 0 {
		return &errs[0], true
	}
	return nil, false
}

func isVolumeNotFoundError(err error) bool {
	if apiErr, ok := extractErrors(err); ok {
		if apiErr.GetType() == "InvalidResource" && apiErr.GetCode() == "5064" {
			return true
		}
	}
	return false
}

func isSnapshotNotFoundError(err error) bool {
	if apiErr, ok := extractErrors(err); ok {
		if apiErr.GetType() == "InvalidResource" && apiErr.GetCode() == "5054" {
			return true
		}
	}
	return false
}

func NewOAPIError(err osc.Errors) OAPIError {
	return OAPIError{
		errors: []osc.Errors{err},
	}
}
