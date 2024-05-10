package api

import "github.com/pkg/errors"

func NewCloudHelper(chType string, opts ...CloudHelperOption) (CloudHelper, error) {
	init, found := cloudHelpersConfigMapping[chType]
	if !found {
		return nil, errors.Errorf("cloud helper of type %q is not supported", chType)
	}
	return init(opts...)
}
