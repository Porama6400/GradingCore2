//go:generate protoc ../../rin.proto --go_out=../../pkg/ --go-grpc_out=../../pkg/ -I ../../

package main

import (
	"GradingCore2/pkg/gateway"
	"GradingCore2/pkg/grading"
	"GradingCore2/pkg/runner"
	"context"
	"encoding/json"
	"log"
	"os"
	"time"
)

type Configuration struct {
	TemplateMap grading.TemplateMap `json:"templates"`
	AmqpUrl     string              `json:"amqp_url"`
	Concurrency int                 `json:"concurrency"`
	TickDelay   int                 `json:"tick_delay"`
}

func LoadConfig() (*Configuration, error) {
	file, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	var Config Configuration
	err = json.Unmarshal(file, &Config)
	if err != nil {
		return nil, err
	}

	return &Config, nil
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		panic(err)
	}

	runnerService, err := runner.NewService()
	if err != nil {
		panic(err)
	}

	err = runnerService.CleanUp(context.Background())
	if err != nil {
		panic(err)
	}

	defer func(runnerService *runner.Service, ctx context.Context) {
		err := runnerService.Shutdown(ctx)
		if err != nil {
			log.Println(err)
		}
	}(runnerService, context.Background())

	gradingService, err := grading.NewService(runnerService, config.TemplateMap)
	if err != nil {
		panic(err)
	}

	gatewayService := gateway.NewService(config.AmqpUrl, config.Concurrency, gradingService)
	go func() {
		for gatewayService.Running {
			err := gatewayService.Tick()
			if err != nil {
				log.Println(err)
			}
			time.Sleep(time.Duration(config.TickDelay) * time.Millisecond)
		}
	}()

	for runnerService.Running {
		time.Sleep(250 * time.Millisecond)
		runnerService.Tick()
	}
	log.Println("shutting down...")
}
