package messages

import "time"

// Release ...
type Release struct {
	ID                     string    `redis:"id"`
	Branch                 string    `redis:"branch"`
	Description            string    `redis:"description"`
	VCSType                string    `redis:"vcs_type"`
	VCSRevision            string    `redis:"vcs_revision"`
	VCSRevisionMessage     string    `redis:"vcs_revision_message"`
	VCSRevisionTime        time.Time `redis:"vcs_revision_time"`
	VCSRevisionAuthorName  string    `redis:"vcs_revision_author_name"`
	VCSRevisionAuthorEmail string    `redis:"vcs_revision_author_email"`
}
