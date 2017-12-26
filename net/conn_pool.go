package net

import (
	"container/heap"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
)

// ConnPoolObject represents an object to be handled by the connection pool
// NOTE: All functions implemented must be concurrency safe!
type ConnPoolObject interface {
	// Close handles any cleanup required for the connection
	// and is called whenever an object has been queued for deletion
	Close() error

	// ShouldDelete indicates whether a ConnPoolObject should be deleted from the pool
	// An example here would be if an http2 connection runs out of streams
	// NOTE: ShouldDelete should resolve quickly, as it has the possibility to deadlock the queue
	ShouldDelete() bool
}

// ConnPoolObjectCanMux is an optimization for connections with muxable connections
// If used then they will be ordered by load value in a min-heap for extraction order
type ConnPoolObjectCanMux interface {
	ConnPoolObject
	Value() <-chan int
}

// ConnPoolContext is what a ConnPool returns from a Get
type ConnPoolContext interface {
	ConnPoolObject() ConnPoolObject
	// NumberOfUsers can be used in determining value for min heap
	NumberOfUsers() int64
	// Done reports to the ConnPool that you are done using this ConnPoolObject.
	// This should be done after ALL operations against the object are resolved
	Done()
}

// connNode is a circulary linked list of connection
// instead of using an array where the earlier connections would
// get much higher load, we'll just run around the loop, so we balance
// in true round-robin fashion
type connNode struct {
	obj ConnPoolObject

	numberUsers int64
	loopback    chan<- *connNode

	// only for use if obj is ConnPoolObjectCanMux
	index int
	value int

	deleted uint32
}

func (cn *connNode) incrementUsers() int64 {
	return atomic.AddInt64(&cn.numberUsers, 1)
}

func (cn *connNode) decrementUsers() int64 {
	return atomic.AddInt64(&cn.numberUsers, -1)
}

func (cn *connNode) ConnPoolObject() ConnPoolObject {
	return cn.obj
}

func (cn *connNode) NumberOfUsers() int64 {
	return atomic.LoadInt64(&cn.numberUsers)
}

func (cn *connNode) Done() {
	cn.decrementUsers()
	cn.loopback <- cn
}

// ConnPool is a fast concurrency safe connection pool structure
type ConnPool interface {
	// Insert returns a no error false case only when we try to insert
	// beyond our max connections limit
	Insert(ConnPoolObject) (bool, error)
	Get() ConnPoolContext
}

// connPool is designed to be a speedy connection pool
// Extensive testing for race conditions is a priority.
// DO NOT edit without updating or checking against existing tests
type connPool struct {
	numConns        int64
	numMuxableConns int64
	maxConns        int64

	logger           *logrus.Entry
	delConnCh        chan *connNode
	noActivityConnCh chan *connNode
	loopbackConnCh   chan *connNode

	muxHeap            muxableConnHeap
	muxHeapEmptyMutex  sync.RWMutex // protects from reading 0 index
	muxHeapChangeMutex sync.Mutex
}

type muxableConnHeap []*connNode

func (mh muxableConnHeap) Len() int { return len(mh) }

func (mh muxableConnHeap) Less(i, j int) bool {
	return mh[i].value < mh[j].value
}

func (mh muxableConnHeap) Swap(i, j int) {
	mh[i], mh[j] = mh[j], mh[i]
	mh[i].index = i
	mh[j].index = j
}

func (mh *muxableConnHeap) Push(x interface{}) {
	n := len(*mh)
	cn := x.(*connNode)
	cn.index = n
	*mh = append(*mh, cn)
}

func (mh *muxableConnHeap) Pop() interface{} {
	old := *mh
	n := len(old)
	cn := old[n-1]
	cn.index = -1 // for safety
	*mh = old[0 : n-1]
	return cn
}

func (pool *connPool) getMuxableConn() <-chan ConnPoolContext {
	cc := make(chan ConnPoolContext)

	go func(cc chan<- ConnPoolContext) {
		pool.muxHeapEmptyMutex.RLock()
		cn := pool.muxHeap[0]
		pool.muxHeapEmptyMutex.RUnlock()
		cc <- cn
	}(cc)

	return cc
}

func (pool *connPool) deleteMuxableConn(cn *connNode) {
	// lets us lazily get min in getMuxableConn
	// without risking index out of bound
	if atomic.AddInt64(&pool.numMuxableConns, -1) == 0 {
		pool.muxHeapEmptyMutex.Lock()
	}

	pool.muxHeapChangeMutex.Lock()
	heap.Remove(&pool.muxHeap, cn.index)
	pool.muxHeapChangeMutex.Unlock()
}

func (pool *connPool) addMuxableConn(cn *connNode) {
	pool.muxHeapChangeMutex.Lock()
	heap.Push(&pool.muxHeap, cn)
	pool.muxHeapChangeMutex.Unlock()

	if atomic.AddInt64(&pool.numMuxableConns, 1) == 1 {
		pool.muxHeapEmptyMutex.Unlock()
	}
}

// NewConnPool creates a new ConnPool
func NewConnPool(logger *logrus.Entry, maxConns int64, initialConns []ConnPoolObject) (ConnPool, error) {
	pool := &connPool{
		maxConns:         maxConns,
		logger:           logger,
		delConnCh:        make(chan *connNode, maxConns),
		noActivityConnCh: make(chan *connNode, maxConns),
		loopbackConnCh:   make(chan *connNode, maxConns),
	}
	pool.muxHeapEmptyMutex.Lock()

	for _, conn := range initialConns {
		ok, err := pool.Insert(conn)
		if !ok {
			return nil, errors.New("Number of initialConns exceeds maxConns")
		}
		if err != nil {
			return nil, errors.Wrapf(err, "Could not insert initial object: %v", conn)
		}
	}

	go pool.handleLoopback()
	go pool.delLoop()
	return pool, nil
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
// the node MUST be in the list currently or be deleted
// NOTE: this is only to be called from the delete chan loop
// NOTE: this only really does anything when a node is muxable
func (pool *connPool) delExistingConn(hc *connNode) {
	// we only need to delete the conn if it was muxable
	// otherwise it simply isn't round tripped from the loopback
	// and is garbage collected
	if _, ok := hc.obj.(ConnPoolObjectCanMux); ok {
		alreadyDeleted := !atomic.CompareAndSwapUint32(&hc.deleted, 0, 1)
		if alreadyDeleted {
			pool.logger.Info("Caught multiple delete request")
			return
		}
		pool.deleteMuxableConn(hc)
	}

	// whether muxable or not we need to decrement the number of total conns
	atomic.AddInt64(&pool.numConns, -1)
}

// Insert adds a new conn to the end of the circulary linked list
func (pool *connPool) Insert(obj ConnPoolObject) (bool, error) {
	// don't insert a new connection when we've maxed out
	if atomic.AddInt64(&pool.numConns, 1) > pool.maxConns {
		// if we've overstepped then decrement and bail
		atomic.AddInt64(&pool.numConns, -1)
		return false, nil
	}

	newConn := &connNode{
		obj:      obj,
		loopback: pool.loopbackConnCh,
	}

	newConn.incrementUsers()
	pool.noActivityConnCh <- newConn

	if mc, ok := obj.(ConnPoolObjectCanMux); ok {
		// hack to get first value
		valC := make(chan int)
		go func(valC chan<- int, mc ConnPoolObjectCanMux) {
			val := <-mc.Value()
			valC <- val
		}(valC, mc)
		newConn.value = <-valC

		pool.addMuxableConn(newConn)
		go pool.handleRevalue(newConn)
	}

	return true, nil
}

func (pool *connPool) handleRevalue(c *connNode) error {
	if mc, ok := c.obj.(ConnPoolObjectCanMux); ok {
		for val := range mc.Value() {
			pool.muxHeapChangeMutex.Lock()
			c.value = val
			heap.Fix(&pool.muxHeap, c.index)
			pool.muxHeapChangeMutex.Unlock()
		}
	} else {
		return errors.New("connNode does not represent a muxable conn")
	}

	return nil
}

// populateAvailable is a run loop which constantly updates the channel of
// non-active connections to be used
func (pool *connPool) handleLoopback() {
	for c := range pool.loopbackConnCh {
		go func(c *connNode) {
			if c.obj.ShouldDelete() {
				pool.delConnCh <- c
				return
			}

			if ok := atomic.CompareAndSwapInt64(&c.numberUsers, 0, 1); ok {
				pool.noActivityConnCh <- c
			}
		}(c)
	}
}

// Get returns a ConnPoolObject
// TODO: Allow set timeout
func (pool *connPool) Get() ConnPoolContext {
	// since select chooses a chan at random when both are populated we want
	// to prioritize the noActicityConnCh first
	select {
	case conn := <-pool.noActivityConnCh:
		return conn
	default:
	}

	select {
	case conn := <-pool.noActivityConnCh:
		return conn
	case conn := <-pool.getMuxableConn():
		return conn
	}
}
