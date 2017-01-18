package wormhole

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	git "gopkg.in/src-d/go-git.v4"

	"github.com/jpillora/backoff"

	handler "github.com/superfly/wormhole/local"
	"github.com/superfly/wormhole/messages"

	log "github.com/Sirupsen/logrus"
)

var (
	localEndpoint  = os.Getenv("LOCAL_ENDPOINT")
	remoteEndpoint = os.Getenv("REMOTE_ENDPOINT")
	cmd            *exec.Cmd
	flyToken       = os.Getenv("FLY_TOKEN")

	releaseIDVar   = os.Getenv("FLY_RELEASE_ID_VAR")
	releaseDescVar = os.Getenv("FLY_RELEASE_DESC_VAR")
	release        = &messages.Release{}
)

func ensureLocalEnvironment() {
	ensureEnvironment()
	if flyToken == "" {
		log.Fatalln("FLY_TOKEN is required, please set this environment variable.")
	}

	if releaseIDVar == "" {
		releaseIDVar = "FLY_RELEASE_ID"
	}

	if releaseDescVar == "" {
		releaseDescVar = "FLY_RELEASE_DESC"
	}

	textFormatter := &log.TextFormatter{FullTimestamp: true}
	log.SetFormatter(textFormatter)
	if remoteEndpoint == "" {
		remoteEndpoint = "wormhole.fly.io:30000"
	}
	computeRelease()
}

func computeRelease() {
	if releaseID := os.Getenv(releaseIDVar); releaseID != "" {
		release.ID = releaseID
	}

	if releaseDesc := os.Getenv(releaseDescVar); releaseDesc != "" {
		release.Description = releaseDesc
	}

	if _, err := os.Stat(".git"); !os.IsNotExist(err) {
		release.VCSType = "git"
		repo, err := git.NewFilesystemRepository(".git")
		if err != nil {
			log.Warnln("Could not open repository:", err)
			return
		}
		ref, err := repo.Head()
		if err != nil {
			log.Warnln("Could not get repo head:", err)
			return
		}

		oid := ref.Hash()
		release.VCSRevision = oid.String()
		tip, err := repo.Commit(oid)
		if err != nil {
			log.Warnln("Could not get current commit:", err)
			return
		}
		author := tip.Author
		release.VCSRevisionAuthorEmail = author.Email
		release.VCSRevisionAuthorName = author.Name
		release.VCSRevisionTime = author.When
		release.VCSRevisionMessage = tip.Message
	}
	if release.ID == "" && release.VCSRevision != "" {
		release.ID = release.VCSRevision
	}
	if release.Description == "" && release.VCSRevisionMessage != "" {
		release.Description = release.VCSRevisionMessage
	}
	log.Println("current release:", release)
}

func runProgram(program string) (localPort string, err error) {
	cs := []string{"/bin/sh", "-c", program}
	cmd = exec.Command(cs[0], cs[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	localPort = os.Getenv("PORT")
	if localPort == "" {
		localPort = "5000"
		cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", 5000))
	} else {
		cmd.Env = os.Environ()
	}
	log.Println("Starting program:", program)
	err = cmd.Start()
	if err != nil {
		log.Println("Failed to start program:", err)
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
				os.Exit(status.ExitStatus())
			}
		}
		return
	}
	go func(cmd *exec.Cmd) {
		err = cmd.Wait()
		if err != nil {
			log.Errorln("Program error:", err)
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					log.Printf("Exit Status: %d", status.ExitStatus())
					os.Exit(status.ExitStatus())
				}
			}
		}
		log.Println("Terminating program", program)
	}(cmd)
	return
}

// StartLocal ...
func StartLocal(ver string) {
	version = ver
	ensureLocalEnvironment()
	args := os.Args[1:]
	if len(args) > 0 {
		localPort, err := runProgram(strings.Join(args, " "))
		if err != nil {
			log.Errorln("Error running program:", err)
			return
		}
		localEndpoint = "127.0.0.1:" + localPort

		for {
			conn, err := net.Dial("tcp", localEndpoint)
			if conn != nil {
				conn.Close()
			}
			if err == nil {
				log.Println("Local server is ready on:", localEndpoint)
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	b := &backoff.Backoff{
		Max: 2 * time.Minute,
	}

	handler := &handler.SSHHandler{
		FlyToken:       flyToken,
		RemoteEndpoint: remoteEndpoint,
		LocalEndpoint:  localEndpoint,
		Release:        release,
		Version:        version,
	}
	for {
		err := handler.InitializeConnection()
		if err != nil {
			log.Errorln("Could not make connection:", err)
			d := b.Duration()
			time.Sleep(d)
			continue
		}
		handleOsSignal(handler)
		b.Reset()
		err = handler.ListenAndServe()
		if err != nil {
			log.Error(err)
		}
	}
}

func handleOsSignal(handler handler.ConnectionHandler) {
	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan)
	go func(signalChan <-chan os.Signal) {
		for sig := range signalChan {
			var exitStatus int
			var exited bool
			if cmd != nil {
				exited, exitStatus, _ = signalProcess(sig)
			} else {
				exitStatus = 0
			}
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				log.Println("Cleaning up local agent.")
				handler.Close()
				os.Exit(int(exitStatus))
			default:
				if cmd != nil && exited {
					os.Exit(int(exitStatus))
				}
			}
		}
	}(signalChan)
}

func signalProcess(sig os.Signal) (exited bool, exitStatus int, err error) {
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
