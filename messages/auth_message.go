package messages

import "time"

// AuthMessage ...
type AuthMessage struct {
	Token   string
	Name    string
	Client  string
	Release *Release
}

// Release ...
type Release struct {
	ID                     string    `redis:"id"`
	Description            string    `redis:"description"`
	VCSType                string    `redis:"vcs_type"`
	VCSRevision            string    `redis:"vcs_revision"`
	VCSRevisionMessage     string    `redis:"vcs_revision_message"`
	VCSRevisionTime        time.Time `redis:"vcs_revision_time"`
	VCSRevisionAuthorName  string    `redis:"vcs_revision_author_name"`
	VCSRevisionAuthorEmail string    `redis:"vcs_revision_author_email"`
}
