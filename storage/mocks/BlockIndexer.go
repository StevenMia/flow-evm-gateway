// Code generated by mockery v2.21.4. DO NOT EDIT.

package mocks

import (
	common "github.com/ethereum/go-ethereum/common"
	mock "github.com/stretchr/testify/mock"

	types "github.com/onflow/flow-go/fvm/evm/types"
)

// BlockIndexer is an autogenerated mock type for the BlockIndexer type
type BlockIndexer struct {
	mock.Mock
}

// FirstHeight provides a mock function with given fields:
func (_m *BlockIndexer) FirstHeight() (uint64, error) {
	ret := _m.Called()

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func() (uint64, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetByHeight provides a mock function with given fields: height
func (_m *BlockIndexer) GetByHeight(height uint64) (*types.Block, error) {
	ret := _m.Called(height)

	var r0 *types.Block
	var r1 error
	if rf, ok := ret.Get(0).(func(uint64) (*types.Block, error)); ok {
		return rf(height)
	}
	if rf, ok := ret.Get(0).(func(uint64) *types.Block); ok {
		r0 = rf(height)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.Block)
		}
	}

	if rf, ok := ret.Get(1).(func(uint64) error); ok {
		r1 = rf(height)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetByID provides a mock function with given fields: ID
func (_m *BlockIndexer) GetByID(ID common.Hash) (*types.Block, error) {
	ret := _m.Called(ID)

	var r0 *types.Block
	var r1 error
	if rf, ok := ret.Get(0).(func(common.Hash) (*types.Block, error)); ok {
		return rf(ID)
	}
	if rf, ok := ret.Get(0).(func(common.Hash) *types.Block); ok {
		r0 = rf(ID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.Block)
		}
	}

	if rf, ok := ret.Get(1).(func(common.Hash) error); ok {
		r1 = rf(ID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LatestHeight provides a mock function with given fields:
func (_m *BlockIndexer) LatestHeight() (uint64, error) {
	ret := _m.Called()

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func() (uint64, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Store provides a mock function with given fields: block
func (_m *BlockIndexer) Store(block *types.Block) error {
	ret := _m.Called(block)

	var r0 error
	if rf, ok := ret.Get(0).(func(*types.Block) error); ok {
		r0 = rf(block)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewBlockIndexer interface {
	mock.TestingT
	Cleanup(func())
}

// NewBlockIndexer creates a new instance of BlockIndexer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewBlockIndexer(t mockConstructorTestingTNewBlockIndexer) *BlockIndexer {
	mock := &BlockIndexer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
