package util

import "maps"

type MutableParameters interface {
	GetParameters() map[string]string
	GetMutableParameters() map[string]string
}

func GetUpdatedParameters(params MutableParameters) map[string]string {
	switch {
	case params.GetMutableParameters() == nil:
		return params.GetParameters()
	case params.GetParameters() == nil:
		return params.GetMutableParameters()
	default:
		p := maps.Clone(params.GetParameters())
		for k, v := range params.GetMutableParameters() {
			p[k] = v
		}
		return p
	}
}
