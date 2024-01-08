package runner

import (
	"GradingCore2/pkg/protorin"
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"sync"
	"time"
)

type Service struct {
	RunningList  []*ContainerInfo
	Runner       *DockerRunner
	PortExternal int
	Running      bool
	Lock         sync.Mutex
}

func NewService(cpuHardLimit float64, memoryHardLimit int64) (*Service, error) {
	runner, err := NewDockerRunner(cpuHardLimit, memoryHardLimit)
	if err != nil {
		return nil, err
	}

	return &Service{
		RunningList:  make([]*ContainerInfo, 1, 16),
		Runner:       runner,
		PortExternal: 8888,
		Running:      true,
		Lock:         sync.Mutex{},
	}, nil
}

func (s *Service) findEmptySlot() int {
	for i := 0; i < len(s.RunningList); i++ {
		if s.RunningList[i] == nil {
			return i
		}
	}

	s.RunningList = append(s.RunningList, nil)
	return len(s.RunningList) - 1
}

func (s *Service) Create(ctx context.Context, template *ContainerTemplate) (*ContainerInfo, error) {
	if s.PortExternal == 0 {
		return nil, errors.New("invalid configuration, tried to set external port to 0")
	}

	s.Lock.Lock()
	startTime := time.Now()
	slot := s.findEmptySlot()
	log.Println("allocated", slot)

	info, err := s.Runner.Start(ctx, &ContainerStartRequest{
		Image:        template.Image,
		Slot:         slot,
		PortInternal: template.PortInternal,
		PortExternal: s.PortExternal + slot,
	})

	if err != nil {
		s.Lock.Unlock()
		return nil, err
	}
	s.RunningList[slot] = info
	s.Lock.Unlock()

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

func (s *Service) Destroy(ctx context.Context, info *ContainerInfo) error {
	slot := info.Request.Slot

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

	err := s.Runner.Stop(ctx, info)
	if err != nil {
		return fmt.Errorf("failed to stop a container %w", err)
	}

	log.Printf("container %s (%d) shutdown ", info.ContainerId, info.Request.Slot)
	s.RunningList[slot] = nil
	return nil
}

func (s *Service) CountRunning() int {

	count := 0
	for _, info := range s.RunningList {
		if info != nil {
			count++
		}
	}
	return count
}

func (s *Service) CleanUp(ctx context.Context) error {
	return s.Runner.CleanUp(ctx)
}

func (s *Service) Shutdown(ctx context.Context) error {
	for i, info := range s.RunningList {
		if s.RunningList[i] == nil {
			continue
		}

		err := s.Destroy(ctx, info)
		if err != nil {
			fmt.Printf("error while stopping container %s %v\n", info.ContainerId, err)
		}
	}

	return nil
}

func (s *Service) Tick() {
	for _, info := range s.RunningList {
		if info == nil {
			continue
		}

		lockSuccess := info.Lock.TryLock()
		if !lockSuccess {
			continue
		}

		if info.WaitForShutdown {
			err := s.Destroy(context.Background(), info)

			if err != nil {
				log.Println("error while shutting down", info.ContainerId, err)
			}
		}
		info.Lock.Unlock()
	}
}
