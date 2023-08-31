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

func hela() {
	request := grading.Request{
		Language:  "go",
		SourceUrl: "base64://cGFja2FnZSBtYWluCgppbXBvcnQgImZtdCIKCmZ1bmMgbWFpbigpIHsKCWZtdC5QcmludGxuKCJIZWxsbyEiKQp9",
		TestCase:  make([]grading.TestCase, 0),
	}

	request.TestCase = append(request.TestCase, grading.TestCase{
		Input:  "base64://IA==",
		Output: "base64://SGVsbG8h",
	})

	jsonBytes, err := json.Marshal(request)
	if err == nil {
		log.Println(string(jsonBytes))
	}
}

type Configuration struct {
	TemplateMap grading.TemplateMap `json:"templates"`
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
	hela()

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

	gatewayService := gateway.NewService("amqp://root:password@58.11.14.67:5672/", 8, gradingService)
	go func() {
		for gatewayService.Running {
			err := gatewayService.Tick()
			if err != nil {
				log.Println(err)
			}
			time.Sleep(250 * time.Millisecond)
		}
	}()

	for runnerService.Running {
		time.Sleep(250 * time.Millisecond)
		runnerService.Tick()
	}
	log.Println("shutting down...")
}
