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
	shouldQueue bool
	sync.Mutex
}

func (c *testConnPoolObj) ShouldQueue() bool {
	return c.shouldQueue
}

func (c *testConnPoolObj) Close() error {
	return nil
}

func (c *testConnPoolObj) ShouldDelete() bool {
	return !c.canContinue
}

func newBaseConnPool(initial []ConnPoolObject, max int) (ConnPool, error) {
	logger := logrus.WithFields(logrus.Fields{"scope": "testing"})
	return NewConnPool(logger, max, initial)
}

func TestInsert(t *testing.T) {
	pool, err := newBaseConnPool([]ConnPoolObject{}, 10)
	assert.NoError(t, err, "Should be no error creating pool")

	obj := &testConnPoolObj{
		canContinue: true,
		shouldQueue: true,
	}
	ok, err := pool.Insert(obj)
	assert.True(t, ok, "Should have room in pool")
	assert.NoError(t, err, "Should have no error inserting into pool")

	objSame := pool.Get()
	assert.Equal(t, obj, objSame, "With just one object in pool we should have only one come back")
}

func TestInsertMulti(t *testing.T) {
	logger := logrus.WithFields(logrus.Fields{"scope": "test_multi"})
	pool, err := newBaseConnPool([]ConnPoolObject{}, 2)
	assert.NoError(t, err, "Should be no error creating pool")

	obj1 := &testConnPoolObj{
		canContinue: true,
		shouldQueue: true,
	}
	logger.Info("Insert 1")
	ok, err := pool.Insert(obj1)
	assert.True(t, ok, "Should have room in pool")
	assert.NoError(t, err, "Should have no error inserting into pool")

	obj2 := &testConnPoolObj{
		canContinue: true,
		shouldQueue: true,
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

	logger.Info("Get 3")
	objGet3 := pool.Get()

	logger.Info("Get 4")
	objGet4 := pool.Get()

	assert.True(t, objGet1 == obj1 || objGet1 == obj2, "Everything we get should be in the set we inserted-1")
	assert.True(t, objGet2 == obj1 || objGet2 == obj2, "Everything we get should be in the set we inserted-2")
	assert.True(t, objGet3 == obj1 || objGet3 == obj2, "Everything we get should be in the set we inserted-3")
	assert.True(t, objGet4 == obj1 || objGet4 == obj2, "Everything we get should be in the set we inserted-4")

	assert.True(t, objGet1 != objGet2 || objGet1 != objGet3 || objGet1 != objGet4, "Use demorgan's to test for set completeness")
}
