package runner

import (
	"GradingCore2/pkg/protorin"
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"time"
)

// Service should remain on main thread at ALL TIME
type Service struct {
	Concurrency  int
	RunningList  []*ContainerInfo
	Runner       *DockerRunner
	PortExternal int
}

func NewService(concurrency int) (*Service, error) {
	runner, err := NewDockerRunner()
	if err != nil {
		return nil, err
	}

	return &Service{
		Concurrency:  concurrency,
		RunningList:  make([]*ContainerInfo, concurrency),
		Runner:       runner,
		PortExternal: 8888,
	}, nil
}

func (r *Service) findEmptySlot() (int, error) {
	for i := 0; i < r.Concurrency; i++ {
		if r.RunningList[i] == nil {
			return i, nil
		}
	}
	return 0, errors.New("no container slot available")
}

func (r *Service) Create(ctx context.Context, template *ContainerTemplate) (*ContainerInfo, error) {
	if r.PortExternal == 0 {
		return nil, errors.New("invalid configuration, tried to set external port to 0")
	}

	startTime := time.Now()

	slot, err := r.findEmptySlot()
	if err != nil {
		return nil, err
	}

	info, err := r.Runner.Start(ctx, &ContainerStartRequest{
		Image:        template.Image,
		Slot:         slot,
		PortInternal: template.PortInternal,
		PortExternal: r.PortExternal + slot,
	})

	if err != nil {
		return nil, err
	}
	r.RunningList[slot] = info

	connectionString := fmt.Sprintf("127.0.0.1:%d", info.Request.PortExternal)
	conn, err := grpc.Dial(connectionString, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("gRPC dial to %s (%s) failed %w", connectionString, info.ContainerId, err)
	}
	info.GrpcConnection = conn
	info.GrpcClient = protorin.NewRinClient(conn)

	log.Printf("container %s (%d) started in %d ms\n", info.ContainerId, info.Request.Slot, time.Now().Sub(startTime).Milliseconds())
	return info, nil
}

func (r *Service) Destroy(ctx context.Context, slot int) error {
	info := r.RunningList[slot]
	r.RunningList[slot] = nil

	if info.GrpcClient != nil {
		_, err := info.GrpcClient.Shutdown(ctx, &protorin.Empty{})
		if err != nil {
			log.Println("error while sending signal shutting down container", info.ContainerId, err)
		}
	}

	if info.GrpcConnection != nil {
		err := info.GrpcConnection.Close()
		if err != nil {
			log.Println("failed to close gRPC connection to a container", info.ContainerId, err)
		}
	}

	if info == nil {
		return fmt.Errorf("tried to stop non-existing container slot %d", slot)
	}

	err := r.Runner.Stop(ctx, info)
	if err != nil {
		return fmt.Errorf("failed to stop a container %w", err)
	}

	log.Printf("container %s (%d) shutdown ", info.ContainerId, info.Request.Slot)
	return nil
}

func (r *Service) CountRunning() int {
	count := 0
	for _, info := range r.RunningList {
		if info != nil {
			count++
		}
	}
	return count
}

func (r *Service) CleanUp(ctx context.Context) error {
	return r.Runner.CleanUp(ctx)
}

func (r *Service) Shutdown(ctx context.Context) {
	for i, info := range r.RunningList {
		if r.RunningList[i] == nil {
			continue
		}

		err := r.Destroy(ctx, i)
		if err != nil {
			fmt.Printf("error while stopping container %s %v\n", info.ContainerId, err)
		}
	}
}
