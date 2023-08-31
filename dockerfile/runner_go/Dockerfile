FROM golang:1.21
WORKDIR /src
COPY cmd cmd
COPY pkg pkg
COPY go.mod go.mod
COPY go.sum go.sum
RUN go build ./cmd/rin

FROM golang:1.21
COPY --from=0 /src/rin /bin/rin
RUN chmod a+x /bin/rin
WORKDIR /src
COPY dockerfile/runner_go/go.mod go.mod
ENV RIN_LISTEN=0.0.0.0:8888
ENV RIN_SOURCE=main.go
ENV RIN_TEST_INPUT=test.txt
ENV RIN_CMD_COMPILE="go build"
ENV RIN_CMD_TEST="./main"
CMD ["/bin/rin"]