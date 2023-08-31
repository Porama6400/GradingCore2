package runner

import (
	"GradingCore2/pkg/protorin"
	"context"
	"google.golang.org/grpc"
	"sync"
)

type ContainerInfo struct {
	ContainerId string
	Port        int

	Request         ContainerStartRequest
	GrpcConnection  *grpc.ClientConn
	GrpcClient      protorin.RinClient
	WaitForShutdown bool
	Lock            sync.RWMutex
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
	Image        string
	PortInternal int
}
