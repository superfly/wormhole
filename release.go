package wormhole

import (
	"fmt"
	"os"
	"strings"

	"github.com/oknoah/wormhole/messages"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

func computeRelease(id, desc, branch string) (*messages.Release, error) {
	release := &messages.Release{}
	if len(id) > 0 {
		release.ID = id
	}

	if len(desc) > 0 {
		release.Description = desc
	}

	if len(branch) > 0 {
		release.Branch = branch
	}

	if _, err := os.Stat(".git"); !os.IsNotExist(err) {
		release.VCSType = "git"
		repo, err := git.PlainOpen(".")
		if err != nil {
			return nil, fmt.Errorf("Could not open repository: %s", err.Error())
		}
		head, err := repo.Head()
		if err != nil {
			return nil, fmt.Errorf("Could not get repo head: %s", err.Error())
		}

		oid := head.Hash()
		release.VCSRevision = oid.String()
		tip, err := repo.CommitObject(oid)
		if err != nil {
			return nil, fmt.Errorf("Could not get current commit: %s", err.Error())
		}

		refs, err := repo.References()
		if err != nil {
			return nil, fmt.Errorf("Could not get current refs: %s", err.Error())
		}

		var branches []string
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name().IsBranch() && head.Hash().String() == ref.Hash().String() {
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

		// TODO: be smarter about branches, and maybe let users override this
		if release.Branch == "" && len(branches) > 0 {
			release.Branch = branches[0]
		}
	}

	if release.ID == "" && release.VCSRevision != "" {
		release.ID = release.VCSRevision
	}
	if release.Description == "" && release.VCSRevisionMessage != "" {
		release.Description = release.VCSRevisionMessage
	}
	return release, nil
}
