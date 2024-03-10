package interactor

import "github.com/docker/go-connections/nat"

type ImageInfo struct {
	ImageRegistryUsername string
	ImageRegisrtyPassword string

	ImageName        string
	ImageExposedPort nat.Port
	ImageControlPort nat.Port
}

type ContainerInfo struct {
	Address     string
	ExposedPort string
}

type ContainerInteractor interface {
	ListContainers() ([]string, error)
	InspectContainer(id string) (ContainerInfo, error)
	CreateContainer() (string, error)
}
