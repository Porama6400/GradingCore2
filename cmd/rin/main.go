package main

import (
	"GradingCore2/pkg/protorin"
	"GradingCore2/pkg/scrubber"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Handler struct {
	protorin.RinServer
	SourcePath     string
	TestInputPath  string
	CompileCommand []string
	TestCommand    []string
	Server         *grpc.Server
}

func (h *Handler) Ping(_ context.Context, _ *protorin.Empty) (*protorin.Empty, error) {
	return &protorin.Empty{}, nil
}

func (h *Handler) Compile(_ context.Context, src *protorin.Source) (*protorin.CompileResult, error) {
	file, err := os.Create(h.SourcePath)
	if err != nil {
		return nil, err
	}

	_, err = file.Write(src.Source)
	if err != nil {
		return nil, err
	}

	command := exec.Command(h.CompileCommand[0], h.CompileCommand[1:]...)
	buffer := bytes.Buffer{}
	command.Stdout = &buffer
	command.Stderr = &buffer
	err = command.Run()
	dataBytes := buffer.Bytes()

	success := err == nil
	result := protorin.CompileResult{Data: dataBytes, Success: &success}

	log.Println(string(dataBytes), err)
	return &result, nil
}

func (h *Handler) Test(_ context.Context, src *protorin.TestContext) (*protorin.TestResult, error) {
	file, err := os.Create(h.TestInputPath)
	if err != nil {
		return nil, err
	}

	_, err = file.Write(src.Source)
	if err != nil {
		return nil, err
	}

	testFile, err := os.Open("test.txt")
	if err != nil {
		return nil, err
	}
	defer func(testFile *os.File) {
		err := testFile.Close()
		if err != nil {
			log.Println(err)
		}
	}(testFile)

	command := exec.Command(h.TestCommand[0], h.TestCommand[1:]...)
	buffer := bytes.Buffer{}
	command.Stdin = testFile
	command.Stdout = &buffer
	command.Stderr = &buffer
	err = command.Run()

	dataBytes := buffer.Bytes()
	dataBytes = scrubber.Scrub(dataBytes)
	hashAlgo := sha256.New()
	hashAlgo.Write(dataBytes)
	hashBytes := hashAlgo.Sum(nil)

	result := protorin.TestResult{Hash: hashBytes}
	if !src.GetOptHashOnly() {
		result.Result = dataBytes
	}

	log.Println(string(dataBytes), err)
	return &result, nil
}

func (h *Handler) Shutdown(context.Context, *protorin.Empty) (*protorin.Empty, error) {
	go func() {
		time.Sleep(1 * time.Second)
		h.Server.Stop()
	}()
	return &protorin.Empty{}, nil
}

func main() {
	handler := Handler{
		SourcePath:     os.Getenv("RIN_SOURCE"),
		TestInputPath:  os.Getenv("RIN_TEST_INPUT"),
		CompileCommand: strings.Split(os.Getenv("RIN_CMD_COMPILE"), " "),
		TestCommand:    strings.Split(os.Getenv("RIN_CMD_TEST"), " "),
	}
	listenAddress := os.Getenv("RIN_LISTEN")

	listen, err := net.Listen("tcp", listenAddress)
	if err != nil {
		panic(err)
	}
	fmt.Println("listening on", listenAddress)
	server := grpc.NewServer()
	handler.Server = server
	protorin.RegisterRinServer(server, &handler)
	err = server.Serve(listen)
	if err != nil {
		panic(err)
	}
}
