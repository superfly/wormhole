package wormhole

import (
	"flag"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	git "srcd.works/go-git.v4"
	"srcd.works/go-git.v4/plumbing"

	"github.com/jpillora/backoff"

	"github.com/superfly/wormhole/local"
	"github.com/superfly/wormhole/messages"
)

const (
	defaultLocalPort      = "5000"
	defaultLocalHost      = "127.0.0.1"
	defaultRemoteEndpoint = "wormhole.fly.io:30000"
	localServerRetry      = 200 * time.Millisecond // how often to retry local server until ready
	maxWormholeBackoff    = 2 * time.Minute        // max backoff between retries to wormhole server
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

	if remoteEndpoint == "" {
		remoteEndpoint = defaultRemoteEndpoint
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

	var branches []string
	if _, err := os.Stat(".git"); !os.IsNotExist(err) {
		release.VCSType = "git"
		repo, err := git.PlainOpen(".")
		if err != nil {
			log.Warnln("Could not open repository:", err)
			return
		}
		head, err := repo.Head()
		if err != nil {
			log.Warnln("Could not get repo head:", err)
			return
		}

		oid := head.Hash()
		release.VCSRevision = oid.String()
		tip, err := repo.Commit(oid)
		if err != nil {
			log.Warnln("Could not get current commit:", err)
			return
		}

		refs, err := repo.References()
		if err != nil {
			log.Warnln("Could not get current refs:", err)
			return
		}
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.IsBranch() && head.Hash().String() == ref.Hash().String() {
				branch := strings.TrimPrefix(ref.Name().String(), "refs/heads/")
				branches = append(branches, branch)
			}
			return nil
		})

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
	// TODO: be smarter about branches, and maybe let users override this
	if len(branches) > 0 {
		release.Branch = branches[0]
	}
	log.Println("Current release:", release)
}

func runProgram(program string) (localPort string, err error) {
	cs := []string{"/bin/sh", "-c", program}
	cmd = exec.Command(cs[0], cs[1:]...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	localPort = os.Getenv("PORT")
	if localPort == "" {
		localPort = defaultLocalPort
		cmd.Env = append(os.Environ(), defaultLocalPort)
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
func StartLocal(cfg *Config) {
	ensureLocalEnvironment()
	args := flag.Args()
	if len(args) > 0 {
		localPort, err := runProgram(strings.Join(args, " "))
		if err != nil {
			log.Errorln("Error running program:", err)
			return
		}
		localEndpoint = defaultLocalHost + ":" + localPort

		for {
			conn, err := net.Dial("tcp", localEndpoint)
			if conn != nil {
				conn.Close()
			}
			if err == nil {
				log.Println("Local server is ready on:", localEndpoint)
				break
			}
			time.Sleep(localServerRetry)
		}
	}

	b := &backoff.Backoff{
		Max: 2 * time.Minute,
	}

	var handler local.ConnectionHandler

	switch cfg.Protocol {
	case SSH:
		handler = &local.SSHHandler{
			FlyToken:       flyToken,
			RemoteEndpoint: remoteEndpoint,
			LocalEndpoint:  localEndpoint,
			Release:        release,
			Version:        cfg.Version,
		}
	case TCP:
		handler = &local.TCPHandler{
			FlyToken:       flyToken,
			RemoteEndpoint: remoteEndpoint,
			LocalEndpoint:  localEndpoint,
			Release:        release,
			Version:        version,
		}

	default:
		log.Fatal("Unknown wormhole transport layer protocol selected.")
	}

	for {
		err := handler.InitializeConnection()
		if err != nil {
			log.Error(err)
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

func handleOsSignal(handler local.ConnectionHandler) {
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
