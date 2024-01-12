package main

import (
	"GradingCore2/pkg/platform"
	"bytes"
	"log"
	"os/exec"
)

func main() {
	command := exec.Command("./test")
	buf := bytes.Buffer{}
	command.Stdout = &buf
	command.Stderr = &buf
	err := command.Run()
	if err != nil {
		log.Println(err)
	}
	log.Println(buf.Bytes())

	result, err := platform.ReportUsage()
	log.Printf("%+v %+v\n", result, err)
}
