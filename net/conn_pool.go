package net

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
)

// connPool is designed to be a speedy connection pool
// Extensive testing for race conditions is a priority.
// DO NOT edit without updating or checking against existing tests
type connPool struct {
	currentConn *connNode
	numConns    int64
	maxConns    int

	logger              *logrus.Entry
	delConnCh           chan *connNode
	goodConnCh          chan *connNode
	insertedWhileNoneCh chan interface{}
	waitingForConn      int32

	// Can't use RWMutex because even when reading we are modifying the currentConn
	// Due to this we will never defer, since this is critical path code. Make SURE you unlock
	// at every exit point. And attempt to have as few exit points as possible
	sync.Mutex
}

// ConnPoolObject represents an object to be handled by the connection pool
// NOTE: All functions implemented must be concurrency safe!
type ConnPoolObject interface {
	// Close handles any cleanup required for the connection
	// and is called whenever an object has been queued for deletion
	Close() error

	// ShouldQueue indicates whether a ConnPoolObject should be queued in the connection pool
	// An example here could be, if an conn-object can't be multiplexed, then return false
	// whenever the object is in-use
	// NOTE: ShouldQueue should resolve quickly, as it has the possibility to deadlock the queue
	ShouldQueue() bool

	// ShouldDelete indicates whether a ConnPoolObject should be deleted from the pool
	// An example here would be if an http2 connection runs out of streams
	// NOTE: ShouldDelete should resolve quickly, as it has the possibility to deadlock the queue
	ShouldDelete() bool
}

// ConnPool is a fast concurrency safe connection pool structure
type ConnPool interface {
	// Insert returns a no error false case only when we try to insert
	// beyond our max connections limit
	Insert(ConnPoolObject) (bool, error)
	Get() ConnPoolObject
}

// NewConnPool creates a new ConnPool
func NewConnPool(logger *logrus.Entry, maxConns int, initialConns []ConnPoolObject) (ConnPool, error) {
	pool := &connPool{
		maxConns:            maxConns,
		logger:              logger,
		delConnCh:           make(chan *connNode, maxConns),
		goodConnCh:          make(chan *connNode, maxConns),
		insertedWhileNoneCh: make(chan interface{}),
		waitingForConn:      0,
	}

	for _, conn := range initialConns {
		ok, err := pool.Insert(conn)
		if !ok {
			return nil, errors.New("Number of initialConns exceeds maxConns")
		}
		if err != nil {
			return nil, errors.Wrapf(err, "Could not insert initial object: %v", conn)
		}
	}

	go pool.populateAvailable()
	go pool.delLoop()
	return pool, nil
}

// connNode is a circulary linked list of connection
// instead of using an array where the earlier connections would
// get much higher load, we'll just run around the loop, so we balance
// in true round-robin fashion
type connNode struct {
	prev *connNode
	next *connNode
	obj  ConnPoolObject

	// deleted is not updated concurrently
	// safe to manipulate unlocked
	deleted uint32
}

func (pool *connPool) delLoop() {
	for {
		delConn := <-pool.delConnCh
		// spawn a new goroutine even though
		// delExistingConn immediately locks the pool
		// because the go runtime does clever things to organize
		// the goroutine mapping to help unlock as often as possible
		go pool.delExistingConn(delConn)
	}
}

// delExistingConn deletes an connNode from the pool
// to be called after all requests/streams have been resolved
// or else the garbage collector could reap the connection before
// all data has been transferred
// the node MUST be in the list currently or be deleted
// NOTE: this is only to be called from the delete chan loop
func (pool *connPool) delExistingConn(hc *connNode) {
	pool.Lock()

	// mark deleted in case
	alreadyDeleted := !atomic.CompareAndSwapUint32(&hc.deleted, 0, 1)
	if alreadyDeleted {
		pool.Unlock()
		pool.logger.Info("Caught multiple delete request")
		return
	}

	if atomic.LoadInt64(&pool.numConns) == 1 {
		if err := hc.obj.Close(); err != nil {
			pool.logger.Errorf("Error cleaning up connection object: %v+", err)
		}
		pool.currentConn = nil
		atomic.StoreInt64(&pool.numConns, 0)

		pool.Unlock()
		return
	}
	if hc == pool.currentConn {
		pool.currentConn = pool.currentConn.next
	}
	hc.prev.next = hc.next
	hc.next.prev = hc.prev
	atomic.AddInt64(&pool.numConns, -1)

	pool.Unlock()
	return
}

// Insert adds a new conn to the end of the circulary linked list
func (pool *connPool) Insert(obj ConnPoolObject) (bool, error) {
	pool.Lock()
	// don't defer here. Defer has perf implications and this is critical path

	// don't insert a new connection when we've maxed out
	if atomic.LoadInt64(&pool.numConns) >= int64(pool.maxConns) && pool.maxConns > 0 {
		pool.Unlock()
		return false, nil
	}

	newConn := &connNode{
		obj: obj,
	}

	if atomic.LoadInt64(&pool.numConns) == 0 {
		newConn.next = newConn
		newConn.prev = newConn
		pool.currentConn = newConn
		atomic.AddInt64(&pool.numConns, 1)

		if atomic.CompareAndSwapInt32(&pool.waitingForConn, 1, 0) {
			pool.insertedWhileNoneCh <- struct{}{}
		}

		pool.Unlock()
		return true, nil
	}

	newConn.next = pool.currentConn
	newConn.prev = pool.currentConn.prev
	pool.currentConn.prev.next = newConn
	pool.currentConn.prev = newConn
	atomic.AddInt64(&pool.numConns, 1)

	pool.Unlock()
	return true, nil
}

// populateAvailable is a run loop which constantly updates the channel of
// connections to be used
func (pool *connPool) populateAvailable() {
	for {

		pool.Lock()

		if pool.currentConn == nil {
			// ensure the Insert knows we're waiting for a connection
			atomic.StoreInt32(&pool.waitingForConn, 1)
			pool.Unlock()

			pool.logger.Warn("No connections in connection pool currently: waiting")

			// wait for insertion
			//<-pool.insertedWhileNoneCh
			<-pool.insertedWhileNoneCh

			pool.logger.Warn("No connections in connection pool currently: got new conn notification")
			// TODO: add more connections in this case. Some sort of queue or signal system
			continue
		}

		if pool.currentConn.obj.ShouldQueue() {
			retConn := pool.currentConn

			// iterate to next conn for next request
			pool.currentConn = pool.currentConn.next

			select {
			case pool.goodConnCh <- retConn:
			default:
			}
			pool.Unlock()
			continue
		} else if pool.currentConn.obj.ShouldDelete() {
			// mark for deletion
			// NOTE: Once a Connection can no longer take a new request
			// it never will be able to again. Therefore a conn will always go down the delete
			// chan. Repeats down the chan are handled in the del method
			pool.delConnCh <- pool.currentConn
			// after marking for deletion, move on to next connection
			pool.currentConn = pool.currentConn.next
		}

		pool.Unlock()
	}
}

// Get returns a ConnPoolObject
// TODO: Allow set timeout
func (pool *connPool) Get() ConnPoolObject {
	conn := <-pool.goodConnCh
	for !conn.obj.ShouldQueue() {
		// ensure state of conn hasn't changed before we return it
		conn = <-pool.goodConnCh
	}
	return conn.obj
}
