// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package testutil

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// RecordingMocks is a pulumi.MockResourceMonitor that keeps the inputs every
// resource was registered with.
//
// Those inputs are where the secret marking of a credential is observable: the
// mock monitor unmarshals a registration with KeepSecrets, so a value wrapped
// by pApi.SecretString arrives here as a secret PropertyValue and an unwrapped
// one does not. Registrations are keyed by type and name rather than by type
// alone, because a stack registers several providers of the same type and
// last-write-wins would quietly assert against the wrong one.
type RecordingMocks struct {
	mu            sync.Mutex
	registrations map[string]resource.PropertyMap

	// StackOutputs are handed back for any stack reference the program
	// resolves, as secrets, which is how a parent stack exports a kubeconfig.
	StackOutputs map[string]string

	// CallResults answer provider function calls (data-source lookups) by
	// token; an unlisted token gets an empty result.
	CallResults map[string]resource.PropertyMap

	// ResourceStates add outputs to the state a registered resource resolves
	// to, keyed by type token. Without them a resource resolves to its own
	// inputs, so every output the program reads back -- a cluster endpoint, a
	// CA certificate -- is empty, and a test that asserts on a value derived
	// from one is asserting on nothing.
	ResourceStates map[string]resource.PropertyMap

	calledTokens []string
}

func NewRecordingMocks() *RecordingMocks {
	return &RecordingMocks{
		registrations:  map[string]resource.PropertyMap{},
		StackOutputs:   map[string]string{},
		CallResults:    map[string]resource.PropertyMap{},
		ResourceStates: map[string]resource.PropertyMap{},
	}
}

func (m *RecordingMocks) NewResource(args sdk.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registrations[registrationKey(args.TypeToken, args.Name)] = args.Inputs

	if args.TypeToken == "pulumi:pulumi:StackReference" {
		outputs := resource.PropertyMap{}
		for name, value := range m.StackOutputs {
			outputs[resource.PropertyKey(name)] = resource.MakeSecret(resource.NewStringProperty(value))
		}
		return args.Name, resource.PropertyMap{
			"name":    resource.NewStringProperty(args.Name),
			"outputs": resource.NewObjectProperty(outputs),
		}, nil
	}
	state := args.Inputs
	if extra, ok := m.ResourceStates[args.TypeToken]; ok {
		state = resource.PropertyMap{}
		for key, value := range args.Inputs {
			state[key] = value
		}
		for key, value := range extra {
			state[key] = value
		}
	}
	return args.Name + "-id", state, nil
}

func (m *RecordingMocks) Call(args sdk.MockCallArgs) (resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calledTokens = append(m.calledTokens, args.Token)
	if result, ok := m.CallResults[args.Token]; ok {
		return result, nil
	}
	return resource.PropertyMap{}, nil
}

// CalledTokens returns every data-source lookup the program made.
//
// An unlisted token gets an empty result rather than an error, because a
// program resolves tokens a test has no opinion about. That makes a stale
// fixture key invisible: the lookup returns nothing, the code under test
// carries on with zero values, and the assertion still passes. A test that
// depends on a fixture should assert it was consumed.
func (m *RecordingMocks) CalledTokens() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calledTokens...)
}

// Inputs returns the inputs of the named resource of the given type.
func (m *RecordingMocks) Inputs(typeToken, name string) (resource.PropertyMap, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inputs, ok := m.registrations[registrationKey(typeToken, name)]
	return inputs, ok
}

// InputsOfType returns the inputs of every registered resource of the given
// type, so a test can assert on all of them without knowing the generated
// resource names.
func (m *RecordingMocks) InputsOfType(typeToken string) []resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []resource.PropertyMap
	for key, inputs := range m.registrations {
		if strings.HasPrefix(key, typeToken+"::") {
			res = append(res, inputs)
		}
	}
	return res
}

func registrationKey(typeToken, name string) string {
	return fmt.Sprintf("%s::%s", typeToken, name)
}
