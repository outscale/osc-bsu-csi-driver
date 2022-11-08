package cloud

import osc "github.com/outscale/osc-sdk-go/v2"

func extractError(err error) (bool, *osc.ErrorResponse) {
	genericError, ok := err.(osc.GenericOpenAPIError)
	if ok {
		errorsResponse, ok := genericError.Model().(osc.ErrorResponse)
		if ok {
			return true, &errorsResponse
		}
		return false, nil
	}
	return false, nil
}

func isVolumeNotFoundError(err error) bool {
	if ok, apirErr := extractError(err); ok {
		if apirErr.GetErrors()[0].GetType() == "InvalidResource" && apirErr.GetErrors()[0].GetCode() == "5064" {
			return true
		}
	}
	return false
}

func isSnapshotNotFoundError(err error) bool {
	if ok, apirErr := extractError(err); ok {
		if apirErr.GetErrors()[0].GetType() == "InvalidResource" && apirErr.GetErrors()[0].GetCode() == "5054" {
			return true
		}
	}
	return false
}
