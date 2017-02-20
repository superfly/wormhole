package messages

import "time"

// Release ...
type Release struct {
	ID                     string    `redis:"id,omitempty"`
	Branch                 string    `redis:"branch,omitempty"`
	Description            string    `redis:"description,omitempty"`
	VCSType                string    `redis:"vcs_type,omitempty"`
	VCSRevision            string    `redis:"vcs_revision,omitempty"`
	VCSRevisionMessage     string    `redis:"vcs_revision_message,omitempty"`
	VCSRevisionTime        time.Time `redis:"vcs_revision_time,omitempty"`
	VCSRevisionAuthorName  string    `redis:"vcs_revision_author_name,omitempty"`
	VCSRevisionAuthorEmail string    `redis:"vcs_revision_author_email,omitempty"`
}
