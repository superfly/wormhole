package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/Sirupsen/logrus"
)

var showTrace = flag.Bool("show-trace", false, "show stack trace after tests finish")

func printTrace() {
	var (
		buf       []byte
		stackSize int
	)
	bufferLen := 16384
	for stackSize == len(buf) {
		buf = make([]byte, bufferLen)
		stackSize = runtime.Stack(buf, true)
		bufferLen *= 2
	}
	buf = buf[:stackSize]
	logrus.Error("===========================STACK TRACE===========================")
	fmt.Println(string(buf))
	logrus.Error("===========================STACK TRACE END=======================")
}

func TestMain(m *testing.M) {
	flag.Parse()
	res := m.Run()
	if *showTrace {
		printTrace()
	}
	os.Exit(res)
}
