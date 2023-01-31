package processor

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/mock"
)

type DockerClientMock struct {
	mock.Mock
}

func (mocked *DockerClientMock) ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error) {
	args := mocked.Called(ctx, options)
	if len(args) > 2 && args.String(2) != "" {
		panic(args.String(2))
	}

	return args.Get(0).([]types.Container), args.Error(1)
}

func (mocked *DockerClientMock) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	args := mocked.Called(ctx, containerID)
	return args.Get(0).(types.ContainerJSON), args.Error(1)
}

func (mocked *DockerClientMock) ImagePull(ctx context.Context, refStr string, options types.ImagePullOptions) (io.ReadCloser, error) {
	args := mocked.Called(ctx, refStr, options)
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (mocked *DockerClientMock) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	args := mocked.Called(ctx, config, hostConfig, networkingConfig, platform, containerName)
	return args.Get(0).(container.ContainerCreateCreatedBody), args.Error(1)
}

func (mocked *DockerClientMock) ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error {
	args := mocked.Called(ctx, containerID, options)
	return args.Error(0)
}

type ReadCloserMock struct {
	mock.Mock
}

func (mocked *ReadCloserMock) Close() error {
	args := mocked.Called()
	return args.Error(0)
}

func (mocked *ReadCloserMock) Read(p []byte) (n int, err error) {
	args := mocked.Called(p)
	return args.Int(0), args.Error(1)
}
