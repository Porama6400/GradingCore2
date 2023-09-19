package grading

import (
	"GradingCore2/pkg/fetcher"
	"GradingCore2/pkg/protorin"
	"GradingCore2/pkg/runner"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"strings"
	"time"
)

type TestCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type ResultCase struct {
	Hash   string `json:"hash"`
	Pass   bool   `json:"pass"`
	Time   int64  `json:"time"`
	Memory int64  `json:"memory"`
}

type Request struct {
	Language  string                 `json:"language"`
	SourceUrl string                 `json:"sourceUrl"`
	TestCase  []TestCase             `json:"test"`
	Metadata  map[string]interface{} `json:"metadata"`
}

type Response struct {
	CompileOutput string                 `json:"compileOutput"`
	Status        StatusCode             `json:"status"`
	Result        []ResultCase           `json:"results"`
	Metadata      map[string]interface{} `json:"metadata"`
}

type TemplateMap map[string]*runner.ContainerTemplate

type Service struct {
	RunnerService *runner.Service
	Fetcher       *fetcher.Service
	TemplateMap   TemplateMap
}

func (r *Response) WrapError(status StatusCode, err error) (*Response, *Error) {
	r.Status = status
	return r, &Error{
		ErrorCode: status,
		Wrap:      err,
	}
}

func NewService(runnerService *runner.Service, templateMap TemplateMap) (*Service, error) {
	return &Service{
		RunnerService: runnerService,
		TemplateMap:   templateMap,
	}, nil
}

func (s *Service) Grade(ctx context.Context, req *Request) (*Response, *Error) {
	timedCtx, cancelFunc := context.WithTimeoutCause(ctx, 10*time.Second, &Error{ErrorCode: StatusFailTimeoutHard, Wrap: nil})
	defer cancelFunc()
	ctx = context.WithValue(ctx, "request", req)
	resp := Response{
		Result:   make([]ResultCase, len(req.TestCase)),
		Status:   StatusUnknown,
		Metadata: req.Metadata,
	}
	req.Language = strings.ToLower(req.Language)

	template := s.TemplateMap[req.Language]
	if template == nil {
		return resp.WrapError(StatusSystemFailMissingImage, fmt.Errorf("template for language %s not found", req.Language))
	}

	info, err := s.RunnerService.Create(timedCtx, template)
	if err != nil {
		return resp.WrapError(StatusSystemFailContainer, err)
	}

	defer func() {
		destroyErr := s.RunnerService.Destroy(ctx, info)
		if destroyErr != nil {
			log.Println("failed to destroy container", info.ContainerId, destroyErr)
		}
	}()

	containerStartSuccess, err := info.Wait(3 * time.Second)
	if !containerStartSuccess {
		return resp.WrapError(StatusSystemFailContainerPing, err)
	}

	source, err := s.Fetcher.Get(req.SourceUrl)
	if err != nil {
		return resp.WrapError(StatusSystemFailFetchFile, err)
	}

	compile, err := info.GrpcClient.Compile(timedCtx, &protorin.Source{Source: source})
	if compile != nil && compile.Data != nil {
		resp.CompileOutput = string(compile.Data)
	}
	if err != nil {
		return resp.WrapError(StatusFailCompilation, err)
	}

	for index, test := range req.TestCase {
		input, err := s.Fetcher.Get(test.Input)
		if err != nil {
			return resp.WrapError(StatusSystemFailFetchFile, err)
		}
		outputExpected, err := s.Fetcher.Get(test.Output)
		if err != nil {
			return resp.WrapError(StatusSystemFailFetchFile, err)
		}

		outputExpectedHashProcessor := sha256.New()
		outputExpectedHashProcessor.Write(outputExpected)
		outputExpectedHash := outputExpectedHashProcessor.Sum(nil)

		hashOnly := false
		timeStart := time.Now()
		data, err := info.GrpcClient.Test(timedCtx, &protorin.TestContext{Source: input, OptHashOnly: &hashOnly})
		if err != nil {
			grpcStatusCode, ok := status.FromError(err)
			if ok && grpcStatusCode.Code() == codes.DeadlineExceeded {
				return resp.WrapError(StatusFailTimeoutHard, err)
			} else {
				return resp.WrapError(StatusSystemFail, err)
			}
		}
		timeElapseMs := time.Now().Sub(timeStart).Milliseconds()

		resultEntry := ResultCase{
			Pass:   bytes.Equal(data.Hash, outputExpectedHash),
			Hash:   base64.StdEncoding.EncodeToString(data.Hash),
			Time:   timeElapseMs,
			Memory: 0,
		}
		resp.Result[index] = resultEntry
		//log.Println(data.Result, data.Hash, timeElapseMs)
		//log.Println(outputExpected)
	}

	resp.Status = StatusCompleted
	return &resp, nil
}
