package main

import (
	"GradingCore2/pkg/protorin"
	"GradingCore2/pkg/runner"
	"context"
	"log"
	"time"
)

func main() {
	service, err := runner.NewService(16)
	if err != nil {
		panic(err)
	}
	err = service.CleanUp(context.Background())
	if err != nil {
		log.Printf("failed to run startup cleanup %v", err)
	}

	defer service.Shutdown(context.Background())

	for i := 0; i < 20; i++ {
		ctx := context.Background()
		finalIteration := i

		con, err := service.Create(ctx, &runner.ContainerTemplate{
			Image:        "rin_go",
			PortInternal: 8888,
		})
		if err != nil {
			log.Printf("error on iteration %d %v\n", finalIteration, err)
			continue
		}

		go func() {
			con.Lock.Lock()
			defer con.Lock.Unlock()

			compile, err := con.GrpcClient.Compile(
				ctx,
				&protorin.Source{
					Source: []byte("package main\n\nfunc main(){\nprintln(\"Hello world!\")\n}"),
				},
			)
			if err != nil {
				log.Printf("failed to compile %d %v %s\n", finalIteration, err, compile)
			} else {
				log.Println("compiled:", finalIteration, string(compile.Data))
			}

			test, err := con.GrpcClient.Test(ctx, &protorin.Source{
				Source: []byte(""),
			})
			if err != nil {
				log.Printf("failed to run test %d %e %s\n", finalIteration, err, test)
			} else {
				log.Println("result:", finalIteration, string(test.Data))
			}

			time.Sleep(10 * time.Second)

			con.WaitForShutdown = true
		}()
	}

	for service.CountRunning() > 0 {
		time.Sleep(1 * time.Second)
		for _, info := range service.RunningList {
			if info == nil {
				continue
			}

			lockSuccess := info.Lock.TryRLock()
			if !lockSuccess {
				continue
			}

			shouldShutdown := info.WaitForShutdown
			info.Lock.RUnlock()
			if shouldShutdown {
				info.Lock.Lock()
				err := service.Destroy(context.Background(), info.Request.Slot)

				if err != nil {
					log.Println("error while shutting down ", info.ContainerId, err)
				}

				info.Lock.Unlock()
			}
		}
	}
}
