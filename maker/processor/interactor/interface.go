package interactor

type ContainerInfo struct {
	Address     string
	ExposedPort string
}

type ContainerInteractor interface {
	ListContainers() ([]string, error)
	InspectContainer(id string) (ContainerInfo, error)
	CreateContainer() (string, error)
}
