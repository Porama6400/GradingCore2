package runner

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"log"
	"strconv"
	"strings"
)

const Prefix = "runner-"

type DockerRunner struct {
	Client          *client.Client
	CpuHardLimit    float64
	MemoryHardLimit int64
}

func NewDockerRunner(cpuHardLimit float64, memoryHardLimit int64) (*DockerRunner, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &DockerRunner{
		Client:          dockerClient,
		CpuHardLimit:    cpuHardLimit,
		MemoryHardLimit: memoryHardLimit,
	}, nil
}

func (r *DockerRunner) createPortConfig(port int, portExt int) (nat.PortSet, nat.PortMap) {
	portString := strconv.FormatInt(int64(port), 10)
	portStringExt := strconv.FormatInt(int64(portExt), 10)
	portName := nat.Port(portString + "/tcp")
	portSet := nat.PortSet{
		portName: struct{}{},
	}

	portSet[portName] = struct{}{}
	portBinding := nat.PortMap{
		portName: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: portStringExt}},
	}

	return portSet, portBinding
}

func (r *DockerRunner) Start(ctx context.Context, request *ContainerStartRequest) (*ContainerInfo, error) {
	slot := request.Slot
	slotName := fmt.Sprintf("%s%d", Prefix, slot)
	portSet, portMap := r.createPortConfig(request.PortInternal, request.PortExternal)
	cfg := container.Config{
		Hostname:     slotName,
		Image:        request.Image,
		ExposedPorts: portSet,
	}
	hostConfig := container.HostConfig{
		Privileged:   false,
		PortBindings: portMap,
		Resources: container.Resources{
			Memory:   r.MemoryHardLimit * 1024,
			NanoCPUs: int64(r.CpuHardLimit * 1e9),
		},
	}

	createResponse, err := r.Client.ContainerCreate(ctx, &cfg, &hostConfig, nil, nil, slotName)
	if err != nil {
		return nil, fmt.Errorf("failed to create a container %w", err)
	}

	containerId := createResponse.ID
	err = r.Client.ContainerStart(ctx, containerId, types.ContainerStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start a container %w", err)
	}
	dockerContainer := ContainerInfo{
		ContainerId: containerId,
		Request:     *request,
	}

	return &dockerContainer, nil
}

func (r *DockerRunner) Stop(ctx context.Context, info *ContainerInfo) error {
	err := r.Client.ContainerRemove(ctx, info.ContainerId, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})

	if err != nil {
		fmt.Printf("failed to remove container %s %v\n", info.ContainerId, err)
	}
	return nil
}

func (r *DockerRunner) CleanUp(ctx context.Context) error {
	list, err := r.Client.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range list {
		names := c.Names
		log.Println(names)
		if len(names) > 0 && strings.HasPrefix(names[0], "/"+Prefix) {
			log.Printf("found stray runner container %s", names[0])

			err := r.Client.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{
				RemoveVolumes: true,
				Force:         true,
			})

			if err != nil {
				log.Printf("failed to delete stray container %v\n", c)
			}
		}
	}

	return nil
}
