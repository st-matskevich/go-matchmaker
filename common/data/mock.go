package data

import (
	"github.com/st-matskevich/go-matchmaker/common"
	"github.com/stretchr/testify/mock"
)

type MockDataProvider struct {
	mock.Mock
}

func (provider *MockDataProvider) Set(req common.RequestBody) (*common.RequestBody, error) {
	args := provider.Called(req)

	var result *common.RequestBody = nil
	if pointer, ok := args.Get(0).(*common.RequestBody); ok {
		result = pointer
	}
	return result, args.Error(1)
}

func (provider *MockDataProvider) ListPush(ID string) error {
	args := provider.Called(ID)
	return args.Error(0)
}

func (provider *MockDataProvider) ListPop() (string, error) {
	args := provider.Called()
	return args.String(0), args.Error(1)
}
