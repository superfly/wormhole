package net

import (
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"sync"
	_ "sync/atomic"
	"testing"
	_ "time"
	_ "unsafe"
)

type testConnPoolObj struct {
	canContinue bool
	sync.Mutex
}

func (c *testConnPoolObj) Close() error {
	return nil
}

func (c *testConnPoolObj) ShouldDelete() bool {
	return !c.canContinue
}

func newBaseConnPool(initial []ConnPoolObject, max int64) (ConnPool, error) {
	logger := logrus.WithFields(logrus.Fields{"scope": "testing"})
	return NewConnPool(logger, max, initial)
}

func TestInsert(t *testing.T) {
	pool, err := newBaseConnPool([]ConnPoolObject{}, 10)
	assert.NoError(t, err, "Should be no error creating pool")

	obj := &testConnPoolObj{
		canContinue: true,
	}
	ok, err := pool.Insert(obj)
	assert.True(t, ok, "Should have room in pool")
	assert.NoError(t, err, "Should have no error inserting into pool")

	objSame := pool.Get()
	assert.Equal(t, obj, objSame.ConnPoolObject(), "With just one object in pool we should have only one come back")
	objSame.Done()

	objSame = pool.Get()
	assert.Equal(t, obj, objSame.ConnPoolObject(), "When we are done with a val it should get queued again on the loopback")
}

func TestInsertMulti(t *testing.T) {
	logger := logrus.WithFields(logrus.Fields{"scope": "test_multi"})
	pool, err := newBaseConnPool([]ConnPoolObject{}, 2)
	assert.NoError(t, err, "Should be no error creating pool")

	obj1 := &testConnPoolObj{
		canContinue: true,
	}
	logger.Info("Insert 1")
	ok, err := pool.Insert(obj1)
	assert.True(t, ok, "Should have room in pool")
	assert.NoError(t, err, "Should have no error inserting into pool")

	obj2 := &testConnPoolObj{
		canContinue: true,
	}
	logger.Info("Insert 2")
	ok, err = pool.Insert(obj2)
	assert.True(t, ok, "Should have room in pool")
	assert.NoError(t, err, "Should have no error inserting into pool")

	// Pool is not guaranteed to return in a particular order soon after inserting
	// This we need to pull of 2xbuffer length to ensure we get the 2 we inserted
	logger.Info("Get 1")
	objGet1 := pool.Get()

	logger.Info("Get 2")
	objGet2 := pool.Get()

	assert.True(t, objGet1.ConnPoolObject() == obj1 || objGet1.ConnPoolObject() == obj2, "Everything we get should be in the set we inserted-1")
	assert.True(t, objGet2.ConnPoolObject() == obj1 || objGet2.ConnPoolObject() == obj2, "Everything we get should be in the set we inserted-2")

}
