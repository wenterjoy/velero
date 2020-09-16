// Code generated by mockery v2.1.0. DO NOT EDIT.

package mocks

import (
	mock "github.com/stretchr/testify/mock"
	velero "github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// DeleteItemAction is an autogenerated mock type for the DeleteItemAction type
type DeleteItemAction struct {
	mock.Mock
}

// AppliesTo provides a mock function with given fields:
func (_m *DeleteItemAction) AppliesTo() (velero.ResourceSelector, error) {
	ret := _m.Called()

	var r0 velero.ResourceSelector
	if rf, ok := ret.Get(0).(func() velero.ResourceSelector); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(velero.ResourceSelector)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Execute provides a mock function with given fields: input
func (_m *DeleteItemAction) Execute(input *velero.DeleteItemActionExecuteInput) error {
	ret := _m.Called(input)

	var r0 error
	if rf, ok := ret.Get(0).(func(*velero.DeleteItemActionExecuteInput) error); ok {
		r0 = rf(input)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}