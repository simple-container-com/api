package tests

import (
	"github.com/simple-container-com/api/pkg/api"
)

var CommonStack = api.Stack{
	Name:    "common",
	Secrets: *CommonSecretsDescriptor,
	Server:  *CommonServerDescriptor,
}

var RefappStack = api.Stack{
	Name:    "refapp",
	Secrets: *CommonSecretsDescriptor,
	Server:  *RefappServerDescriptor,
	Client:  *RefappClientDescriptor,
}
