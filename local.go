package wormhole

import (
	"flag"
	"net"
	"os"
	"strings"
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
	port           = os.Getenv("PORT")
	remoteEndpoint = os.Getenv("REMOTE_ENDPOINT")
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

	if localEndpoint == "" {
		if port == "" {
			localEndpoint = defaultLocalHost + ":" + defaultLocalPort
		} else {
			localEndpoint = defaultLocalHost + ":" + port
		}
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

// StartLocal ...
func StartLocal(cfg *Config) {
	ensureLocalEnvironment()

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
	args := flag.Args()
	if len(args) > 0 {
		cmd := strings.Join(args, " ")
		process := NewProcess(cmd, handler)
		err := process.Run()
		if err != nil {
			log.Fatalf("Error running program: %s", err.Error())
			return
		}

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

	for {
		err := handler.InitializeConnection()
		if err != nil {
			log.Error(err)
			d := b.Duration()
			time.Sleep(d)
			continue
		}
		b.Reset()
		err = handler.ListenAndServe()
		if err != nil {
			log.Error(err)
		}
	}
}
