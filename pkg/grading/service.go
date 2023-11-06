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

type RequestSettings struct {
	TimeLimit   int `json:"timeLimit"`
	MemoryLimit int `json:"memoryLimit"`
}

type Request struct {
	Language  string                 `json:"language"`
	SourceUrl string                 `json:"sourceUrl"`
	TestCase  []TestCase             `json:"test"`
	Settings  RequestSettings        `json:"settings"`
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

func (r *Response) WrapStatus(status StatusCode) (*Response, *Error) {
	r.Status = status
	return r, nil
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

const TimeLimitHard = 5 * time.Second
const SystemTimeLimit = 10 * time.Second

func (s *Service) Grade(ctx context.Context, req *Request) (*Response, *Error) {

	caseTimeLimitSoft := time.Duration(req.Settings.TimeLimit) * time.Millisecond
	if caseTimeLimitSoft == 0 {
		caseTimeLimitSoft = TimeLimitHard
	}

	caseTimeLimitHard := caseTimeLimitSoft + time.Second
	if caseTimeLimitHard > TimeLimitHard {
		caseTimeLimitHard = TimeLimitHard
	}
	log.Println("grading", req.SourceUrl, " limits: ", caseTimeLimitSoft, caseTimeLimitHard)

	timedSystemContext, cancelTimedSetupContext := context.WithTimeout(ctx, SystemTimeLimit)
	defer cancelTimedSetupContext()

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

	runnerContainer, err := s.RunnerService.Create(timedSystemContext, template)
	if err != nil {
		return resp.WrapError(StatusSystemFailContainer, err)
	}

	defer func() {
		destroyErr := s.RunnerService.Destroy(ctx, runnerContainer)
		if destroyErr != nil {
			log.Println("failed to destroy container", runnerContainer.ContainerId, destroyErr)
		}
	}()

	containerStartSuccess, err := runnerContainer.Wait(SystemTimeLimit)
	if !containerStartSuccess {
		return resp.WrapError(StatusSystemFailContainerPing, err)
	}

	source, err := s.Fetcher.Get(req.SourceUrl)
	if err != nil {
		return resp.WrapError(StatusSystemFailFetchFile, err)
	}

	compile, err := runnerContainer.GrpcClient.Compile(timedSystemContext, &protorin.Source{Source: source})
	if compile != nil && compile.Data != nil {
		resp.CompileOutput = string(compile.Data)
	}
	if err != nil || !*compile.Success {
		fromError, ok := status.FromError(err)
		if ok && fromError.Code() == codes.DeadlineExceeded {
			return resp.WrapError(StatusFailCompilationTimeout, err)
		} else {
			return resp.WrapStatus(StatusFailCompilation)
		}
	}

	caseTimeExceedAtleastOnce := false
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

		timedCaseContext, cancelTimedCaseContext := context.WithTimeoutCause(ctx, caseTimeLimitHard, &Error{ErrorCode: StatusFailTimeoutHard, Wrap: nil})

		hashOnly := false
		timeStart := time.Now()
		data, err := runnerContainer.GrpcClient.Test(timedCaseContext, &protorin.TestContext{Source: input, OptHashOnly: &hashOnly})
		cancelTimedCaseContext()

		if err != nil {
			grpcStatusCode, ok := status.FromError(err)
			if ok && grpcStatusCode.Code() == codes.DeadlineExceeded {
				return resp.WrapStatus(StatusFailTimeoutHard)
			} else {
				return resp.WrapError(StatusSystemFail, err)
			}
		}
		timeElapse := time.Now().Sub(timeStart)
		caseTimeExceed := timeElapse > caseTimeLimitSoft
		caseTimeExceedAtleastOnce = caseTimeExceedAtleastOnce || caseTimeExceed

		resultEntry := ResultCase{
			Pass:   !caseTimeExceed && bytes.Equal(data.Hash, outputExpectedHash),
			Hash:   base64.StdEncoding.EncodeToString(data.Hash),
			Time:   timeElapse.Milliseconds(),
			Memory: 0,
		}
		resp.Result[index] = resultEntry
	}

	if caseTimeExceedAtleastOnce {
		resp.Status = StatusFailTimeout
	} else {
		resp.Status = StatusCompleted
	}
	return &resp, nil
}
