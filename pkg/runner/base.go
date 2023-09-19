package runner

import (
	"GradingCore2/pkg/protorin"
	"context"
	"errors"
	"google.golang.org/grpc"
	"sync"
	"time"
)

type ContainerInfo struct {
	ContainerId string
	Port        int

	Request         ContainerStartRequest
	GrpcConnection  *grpc.ClientConn
	GrpcClient      protorin.RinClient
	WaitForShutdown bool
	Lock            sync.Mutex
}

func (c *ContainerInfo) Wait(timeLimit time.Duration) (bool, error) {
	background := context.Background()
	threshold := time.Now().Add(timeLimit)
	var err error

	if c.GrpcClient == nil {
		return false, errors.New("gRPC client is nil")
	}

	for time.Now().Before(threshold) {
		_, err = c.GrpcClient.Ping(background, &protorin.Empty{})
		if err == nil {
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return false, err
}

type ContainerStartRequest struct {
	Slot         int
	Image        string
	PortInternal int
	PortExternal int
}

type Runner interface {
	Start(ctx context.Context, request *ContainerStartRequest) (*ContainerInfo, error)
	Stop(ctx context.Context, info *ContainerInfo) error
}

type ContainerTemplate struct {
	Id           string `json:"id"`
	Image        string `json:"image"`
	PortInternal int    `json:"portInternal"`
}
