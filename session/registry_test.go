package session

import (
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"testing"
)

func TestRegistry_CRUD(t *testing.T) {
	sess := &baseSession{
		id: "sess-1",
	}
	r := NewRegistry(log.New())

	assert.Nil(t, r.GetSession(sess.ID()), "should be initially empty")

	r.AddSession(sess)

	assert.Equal(t, sess, r.GetSession(sess.ID()), "sessions should be matching")

	r.RemoveSession(sess)

	assert.Nil(t, r.GetSession(sess.ID()), "should be removed")
}
