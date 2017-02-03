package wormhole

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

type Process struct {
	cmd    *exec.Cmd
	closer io.Closer
}

func NewProcess(program string, closer io.Closer) *Process {
	cs := []string{"/bin/sh", "-c", program}
	cmd := exec.Command(cs[0], cs[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return &Process{cmd: cmd, closer: closer}
}

func (p *Process) Run() (err error) {
	p.handleOsSignal()

	log.Println("Starting program:", p.cmd.Path)
	if err = p.cmd.Start(); err != nil {
		log.Println("Failed to start program:", err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
				os.Exit(status.ExitStatus())
			}
		}
	}
	go wait(p.cmd)
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
				log.Println("Cleaning up local agent.")
				p.closer.Close()
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

func wait(cmd *exec.Cmd) {
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
