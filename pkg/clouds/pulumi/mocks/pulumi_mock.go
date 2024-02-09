// Code generated by mockery v2.39.1. DO NOT EDIT.

package pulumi_mocks

import (
	context "context"
	api "github.com/simple-container-com/api/pkg/api"

	mock "github.com/stretchr/testify/mock"
)

// PulumiMock is an autogenerated mock type for the Pulumi type
type PulumiMock struct {
	mock.Mock
}

// ProvisionStack provides a mock function with given fields: ctx, cfg, pubKey, stack
func (_m *PulumiMock) ProvisionStack(ctx context.Context, cfg *api.ConfigFile, pubKey string, stack api.Stack) error {
	ret := _m.Called(ctx, cfg, pubKey, stack)

	if len(ret) == 0 {
		panic("no return value specified for ProvisionStack")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *api.ConfigFile, string, api.Stack) error); ok {
		r0 = rf(ctx, cfg, pubKey, stack)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewPulumiMock creates a new instance of PulumiMock. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewPulumiMock(t interface {
	mock.TestingT
	Cleanup(func())
}) *PulumiMock {
	mock := &PulumiMock{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
