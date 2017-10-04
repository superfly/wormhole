package wormhole

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

// Process is a wrapper around external program
// It handles execution and shutdown of a program and can notify an optional io.Closer
// when a program is terminated.
type Process struct {
	cmd    *exec.Cmd
	closer io.Closer
	logger *logrus.Entry
}

// NewProcess returns the Process to execute a named program
func NewProcess(logger *logrus.Logger, program string, closer io.Closer) *Process {
	cs := []string{"/bin/sh", "-c", program}
	cmd := exec.Command(cs[0], cs[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return &Process{cmd: cmd, closer: closer, logger: logger.WithFields(logrus.Fields{"prefix": "process"})}
}

// Run starts the specified command and returns
// without waiting for the program to complete.
// It sets up a os.Signal listener and passes SIGNINT and SIGTERM to the running program.
// It also closer the Closer if present.
func (p *Process) Run() (err error) {
	p.handleOsSignal()

	p.logger.Println("Starting program:", p.cmd.Path)
	if err = p.cmd.Start(); err != nil {
		p.logger.Println("Failed to start program:", err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				p.logger.Printf("Exit Status: %d", status.ExitStatus())
				os.Exit(status.ExitStatus())
			}
		}
	}
	go wait(p.cmd, p.logger)
	return
}

func (p *Process) handleOsSignal() {
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan)
	go func(signalChan <-chan os.Signal) {
		for sig := range signalChan {
			var exitStatus int
			var exited bool
			if p.cmd != nil {
				exited, exitStatus, _ = signalProcess(p.cmd, sig)
			} else {
				exitStatus = 0
			}
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				p.logger.Println("Cleaning up local agent.")
				if p.closer != nil {
					p.closer.Close()
				}
				os.Exit(int(exitStatus))
			default:
				if p.cmd != nil && exited {
					os.Exit(int(exitStatus))
				}
			}
		}
	}(signalChan)
}

func signalProcess(cmd *exec.Cmd, sig os.Signal) (exited bool, exitStatus int, err error) {
	exited = false
	err = cmd.Process.Signal(sig)
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			exited = true
			status, _ := exiterr.Sys().(syscall.WaitStatus)
			exitStatus = status.ExitStatus()
		}
	}
	return
}

func wait(cmd *exec.Cmd, log *logrus.Entry) {
	if err := cmd.Wait(); err != nil {
		log.Errorln("Program error:", err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
				os.Exit(status.ExitStatus())
			}
		}
	}
	log.Println("Terminating program", cmd.Path)
}
