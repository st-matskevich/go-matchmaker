package interactor

import (
	"github.com/stretchr/testify/mock"
)

type MockInteractor struct {
	mock.Mock
}

func (mocked *MockInteractor) ListContainers() ([]string, error) {
	args := mocked.Called()
	if len(args) > 2 && args.String(2) != "" {
		panic(args.String(2))
	}

	return args.Get(0).([]string), args.Error(1)
}

func (mocked *MockInteractor) InspectContainer(id string) (ContainerInfo, error) {
	args := mocked.Called(id)
	return args.Get(0).(ContainerInfo), args.Error(1)
}

func (mocked *MockInteractor) CreateContainer() (string, error) {
	args := mocked.Called()
	return args.String(0), args.Error(1)
}
