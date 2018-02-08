package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionStore_RequiresClientAuth(t *testing.T) {
	store, err := testRedisStore()
	if err != nil {
		t.Fatal("Couldn't initialize SessionStore: ", err)
	}

	testRedis.HSet("backend:1", "client_auth_disabled", "true")
	testRedis.HSet("backend:3", "client_auth_disabled", "false")
	testRedis.HSet("backend:5", "id", "5")

	auth, err := store.BackendRequiresClientAuth("1")
	assert.False(t, auth)
	assert.NoError(t, err)

	auth, err = store.BackendRequiresClientAuth("3")
	assert.True(t, auth)
	assert.NoError(t, err)

	auth, err = store.BackendRequiresClientAuth("5")
	assert.True(t, auth)
	assert.NoError(t, err)

	auth, err = store.BackendRequiresClientAuth("badid")
	assert.True(t, auth)
	assert.NoError(t, err)
}

func TestSessionStore_ClientCAs(t *testing.T) {
	store, err := testRedisStore()
	if err != nil {
		t.Fatal("Couldn't initialize SessionStore: ", err)
	}

	testRedis.HSet("backend:1", "client_auth_chain", "fullchain_1")
	testRedis.HSet("backend:5", "id", "5")

	chain, err := store.GetClientCAs("1")
	assert.Equal(t, []byte("fullchain_1"), chain)
	assert.NoError(t, err)

	chain, err = store.GetClientCAs("5")
	assert.Nil(t, chain)
	assert.Error(t, err)

	chain, err = store.GetClientCAs("badid")
	assert.Nil(t, chain)
	assert.Error(t, err)
}

func TestSessionStore_ValidCertificate(t *testing.T) {
	store, err := testRedisStore()
	if err != nil {
		t.Fatal("Couldn't initialize SessionStore: ", err)
	}

	testRedis.SetAdd("backend:1:valid_certificates", "fingerprint_1", "fingerprint_3")

	valid, err := store.ValidCertificate("1", "fingerprint_1")
	assert.True(t, valid)
	assert.NoError(t, err)

	valid, err = store.ValidCertificate("1", "fingerprint_notexistant")
	assert.False(t, valid)
	assert.NoError(t, err)

	valid, err = store.ValidCertificate("5", "fingerprint_1")
	assert.False(t, valid)
	assert.NoError(t, err)

	valid, err = store.ValidCertificate("5", "fingerprint_notexistant")
	assert.False(t, valid)
	assert.NoError(t, err)
}

func testRedisStore() (*RedisStore, error) {
	conn := redisPool.Get()
	defer conn.Close()

	if _, err := conn.Do("FLUSHALL"); err != nil {
		return nil, err
	}

	return NewRedisStore(redisPool), nil
}
