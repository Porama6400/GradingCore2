package grading

import (
	"GradingCore2/pkg/fetcher"
	"GradingCore2/pkg/protorin"
	"GradingCore2/pkg/runner"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"
)

type Request struct {
	Language  string            `json:"language"`
	SourceUrl string            `json:"sourceUrl"`
	TestCase  []TestCase        `json:"test"`
	Metadata  map[string]string `json:"metadata"`
}

type TestCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type ResultCase struct {
	Hash string `json:"hash"`
	Pass bool   `json:"pass"`
	Time int64  `json:"time"`
}

type Response struct {
	CompileOutput []byte            `json:"compileOutput"`
	Result        []ResultCase      `json:"result"`
	Metadata      map[string]string `json:"metadata"`
}

type TemplateMap map[string]*runner.ContainerTemplate

type Service struct {
	RunnerService *runner.Service
	Fetcher       *fetcher.Service
	TemplateMap   TemplateMap
}

func NewService(runnerService *runner.Service, templateMap TemplateMap) (*Service, error) {
	return &Service{
		RunnerService: runnerService,
		TemplateMap:   templateMap,
	}, nil
}

func (s *Service) Grade(ctx context.Context, req *Request) (*Response, error) {
	ctx = context.WithValue(ctx, "request", req)
	resp := Response{
		Result:   make([]ResultCase, len(req.TestCase)),
		Metadata: req.Metadata,
	}
	req.Language = strings.ToLower(req.Language)

	template := s.TemplateMap[req.Language]
	if template == nil {
		return nil, fmt.Errorf("template for language %s not found", req.Language)
	}

	info, err := s.RunnerService.Create(ctx, template)
	if err != nil {
		return nil, err
	}

	defer func() {
		destroyErr := s.RunnerService.Destroy(ctx, info)
		if destroyErr != nil {
			log.Println("failed to destroy container", info.ContainerId, destroyErr)
		}
	}()

	source, err := s.Fetcher.Get(req.SourceUrl)
	if err != nil {
		return nil, err
	}

	compile, err := info.GrpcClient.Compile(ctx, &protorin.Source{Source: source})
	if compile != nil {
		resp.CompileOutput = compile.Data
	}
	if err != nil {
		return nil, err
	}

	for index, test := range req.TestCase {
		input, err := s.Fetcher.Get(test.Input)
		if err != nil {
			return nil, err
		}
		output, err := s.Fetcher.Get(test.Output)
		if err != nil {
			return nil, err
		}

		hashOnly := true
		timeStart := time.Now()
		data, err := info.GrpcClient.Test(ctx, &protorin.TestContext{Source: input, OptHashOnly: &hashOnly})
		if err != nil {
			return nil, err
		}
		timeElapseMs := time.Now().Sub(timeStart).Milliseconds()

		resultEntry := ResultCase{
			Pass: bytes.Equal(data.Result, output),
			Hash: base64.StdEncoding.EncodeToString(data.Hash),
			Time: timeElapseMs,
		}
		resp.Result[index] = resultEntry
		log.Println(data.Result, output, data.Hash, timeElapseMs)
		log.Println(string(data.Result), string(output))
	}

	return &resp, nil
}
