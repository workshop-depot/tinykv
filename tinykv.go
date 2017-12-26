package tinykv

import (
	"fmt"
	"sync"
	"time"
	// "github.com/pkg/errors"
)

//-----------------------------------------------------------------------------

type timeout struct {
	expiresAt    time.Time
	expiresAfter time.Duration
	isSliding    bool
	key          string
}

func newTimeout(
	key string,
	expiresAfter time.Duration,
	isSliding bool) *timeout {
	return &timeout{
		expiresAt:    time.Now().Add(expiresAfter),
		expiresAfter: expiresAfter,
		isSliding:    isSliding,
		key:          key,
	}
}

func (to *timeout) slide() {
	if to == nil {
		return
	}
	if !to.isSliding {
		return
	}
	if to.expiresAfter <= 0 {
		return
	}
	to.expiresAt = time.Now().Add(to.expiresAfter)
}

func (to *timeout) expired() bool {
	if to == nil {
		return false
	}
	return time.Now().After(to.expiresAt)
}

//-----------------------------------------------------------------------------

// timeout heap
type th []*timeout

func (h th) Len() int           { return len(h) }
func (h th) Less(i, j int) bool { return h[i].expiresAt.After(h[j].expiresAt) }
func (h th) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *th) Push(x tohVal)     { *h = append(*h, x) }
func (h *th) Pop() tohVal {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

//-----------------------------------------------------------------------------

type entry struct {
	*timeout
	value interface{}
}

//-----------------------------------------------------------------------------

// KV is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type KV interface {
	Delete(k string)
	Get(k string) (v interface{}, ok bool)
	Put(k string, v interface{}, options ...PutOption) error
	Take(k string) (v interface{}, ok bool)
	Stop()
}

//-----------------------------------------------------------------------------

type putOpt struct {
	expiresAfter time.Duration
	isSliding    bool
	cas          func(interface{}, bool) bool
}

// PutOption extra options for put
type PutOption func(*putOpt)

// ExpiresAfter entry will expire after this time
func ExpiresAfter(expiresAfter time.Duration) PutOption {
	return func(opt *putOpt) {
		opt.expiresAfter = expiresAfter
	}
}

// IsSliding sets if the entry would get expired in a sliding manner
func IsSliding(isSliding bool) PutOption {
	return func(opt *putOpt) {
		opt.isSliding = isSliding
	}
}

// CAS for performing a compare and swap
func CAS(cas func(oldValue interface{}, found bool) bool) PutOption {
	return func(opt *putOpt) {
		opt.cas = cas
	}
}

//-----------------------------------------------------------------------------

// store is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type store struct {
	onExpire func(k string, v interface{})

	stop               chan struct{}
	stopOnce           sync.Once
	expirationInterval time.Duration
	mx                 sync.Mutex
	kv                 map[string]*entry
	heap               th
}

// New creates a new *store, onExpire is for notification (must be fast).
func New(expirationInterval time.Duration, onExpire ...func(k string, v interface{})) KV {
	if expirationInterval <= 0 {
		expirationInterval = time.Second * 20
	}
	res := &store{
		stop:               make(chan struct{}),
		kv:                 make(map[string]*entry),
		expirationInterval: expirationInterval,
		heap:               th{},
	}
	if len(onExpire) > 0 && onExpire[0] != nil {
		res.onExpire = onExpire[0]
	}
	go res.expireLoop()
	return res
}

// Stop stops the goroutine
func (kv *store) Stop() {
	kv.stopOnce.Do(func() { close(kv.stop) })
}

// Delete deletes an entry
func (kv *store) Delete(k string) {
	kv.mx.Lock()
	defer kv.mx.Unlock()
	delete(kv.kv, k)
}

// Get gets an entry from KV store
// and if a sliding timeout is set, it will be slided
func (kv *store) Get(k string) (interface{}, bool) {
	kv.mx.Lock()
	defer kv.mx.Unlock()

	e, ok := kv.kv[k]
	if !ok {
		return nil, ok
	}
	e.slide()
	if e.expired() {
		go notifyExpirations(map[string]interface{}{k: e.value}, kv.onExpire)
		delete(kv.kv, k)
		return nil, false
	}
	return e.value, ok
}

// Put puts an entry inside kv store with provided options
func (kv *store) Put(k string, v interface{}, options ...PutOption) error {
	opt := &putOpt{}
	for _, v := range options {
		v(opt)
	}
	e := &entry{
		value: v,
	}
	kv.mx.Lock()
	defer kv.mx.Unlock()
	if opt.expiresAfter > 0 {
		e.timeout = newTimeout(k, opt.expiresAfter, opt.isSliding)
		timeheapPush(&kv.heap, e.timeout)
	}
	if opt.cas != nil {
		return kv.cas(k, e, opt.cas)
	}
	kv.kv[k] = e
	return nil
}

func (kv *store) cas(k string, e *entry, casFunc func(interface{}, bool) bool) error {
	old, ok := kv.kv[k]
	var oldValue interface{}
	if ok && old != nil {
		oldValue = old.value
	}
	if !casFunc(oldValue, ok) {
		return ErrCASCond
	}
	if ok && old != nil {
		old.slide()
		old.value = e.value
		e = old
	}
	kv.kv[k] = e
	return nil
}

// Take takes an entry out of kv store
func (kv *store) Take(k string) (interface{}, bool) {
	kv.mx.Lock()
	defer kv.mx.Unlock()
	e, ok := kv.kv[k]
	if ok {
		delete(kv.kv, k)
		return e.value, ok
	}
	return nil, ok
}

//-----------------------------------------------------------------------------

func (kv *store) expireLoop() {
	interval := kv.expirationInterval
	expireTime := time.NewTimer(interval)
	for {
		select {
		case <-kv.stop:
			return
		case <-expireTime.C:
			v := kv.expireFunc()
			if v < 0 {
				v = -1 * v
			}
			if v > 0 && v < kv.expirationInterval {
				interval = v
			}
			expireTime.Reset(interval)
		}
	}
}

func (kv *store) expireFunc() time.Duration {
	kv.mx.Lock()
	defer kv.mx.Unlock()

	var interval time.Duration
	if len(kv.heap) == 0 {
		return interval
	}
	expired := make(map[string]interface{})
	c := -1
	for {
		if len(kv.heap) == 0 {
			break
		}
		c++
		if c >= len(kv.heap) {
			break
		}
		last := kv.heap[0]
		entry, ok := kv.kv[last.key]
		if !ok {
			timeheapPop(&kv.heap)
			continue
		}
		if !last.expired() {
			interval = last.expiresAt.Sub(time.Now())
			if interval < 0 {
				interval = last.expiresAfter
			}
			break
		}
		last = timeheapPop(&kv.heap)
		if ok {
			expired[last.key] = entry.value
		}
	}
	for k := range expired {
		delete(kv.kv, k)
	}
	go notifyExpirations(expired, kv.onExpire)
	if interval == 0 && len(kv.heap) > 0 {
		last := kv.heap[len(kv.heap)-1]
		interval = last.expiresAt.Sub(time.Now())
		if interval < 0 {
			interval = last.expiresAfter
		}
	}
	return interval
}

func notifyExpirations(
	expired map[string]interface{},
	onExpire func(k string, v interface{})) {
	if onExpire == nil {
		return
	}
	for k, v := range expired {
		k, v := k, v
		try(func() error {
			onExpire(k, v)
			return nil
		})
	}
}

//-----------------------------------------------------------------------------

// errors
var (
	ErrCASCond = errorf("CAS COND FAILED")
)

//-----------------------------------------------------------------------------

type sentinelErr string

func (v sentinelErr) Error() string { return string(v) }
func errorf(format string, a ...interface{}) error {
	return sentinelErr(fmt.Sprintf(format, a...))
}

//-----------------------------------------------------------------------------
