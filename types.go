package tinykv

import (
	"context"
	"sync"
	"time"

	"github.com/dc0d/supervisor"
)

//-----------------------------------------------------------------------------

// Option for store setup
type Option func(*store)

// OnExpire sets the onExpire func
func OnExpire(onExpire func(k, v interface{})) Option {
	return func(kv *store) { kv.onExpire = onExpire }
}

// Context sets the context
func Context(ctx context.Context) Option {
	return func(kv *store) { kv.ctx = ctx }
}

// ExpirationInterval sets the expirationInterval for its agent (goroutine)
func ExpirationInterval(expirationInterval time.Duration) Option {
	return func(kv *store) { kv.expirationInterval = expirationInterval }
}

//-----------------------------------------------------------------------------

// New creates a new *store
func New(options ...Option) KV {
	res := &store{
		kv: make(map[interface{}]*entry),
	}
	for _, opt := range options {
		opt(res)
	}
	if res.expirationInterval <= 0 {
		res.expirationInterval = 30 * time.Second
	}
	go res.expireLoop()
	return res
}

//-----------------------------------------------------------------------------

// KV is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type KV interface {
	CAS(k, v interface{}, cond func(interface{}, error) bool, options ...PutOption) error
	Delete(k interface{})
	Get(k interface{}) (v interface{}, ok bool)
	Put(k, v interface{}, options ...PutOption)
	Take(k interface{}) (v interface{}, ok bool)
}

//-----------------------------------------------------------------------------

// Get .
func (kv *store) Get(k interface{}) (interface{}, bool) {
	kv.mx.Lock()
	defer kv.mx.Unlock()

	e, ok := kv.kv[k]
	if !ok {
		return nil, ok
	}
	e.slide()
	return e.value, ok
}

//-----------------------------------------------------------------------------

type putConf struct {
	expiresAfter time.Duration
	isSliding    bool
}

// PutOption extra options for put
type PutOption func(putConf) putConf

// ExpiresAfter entry will expire after this time
func ExpiresAfter(expiresAfter time.Duration) PutOption {
	return func(opt putConf) putConf {
		opt.expiresAfter = expiresAfter
		return opt
	}
}

// IsSliding sets if the entry would get expired in a sliding manner
func IsSliding(isSliding bool) PutOption {
	return func(opt putConf) putConf {
		opt.isSliding = isSliding
		return opt
	}
}

// Put a (k, v) tuple in the store store
func (kv *store) Put(k, v interface{}, options ...PutOption) {
	var pc putConf
	for _, opt := range options {
		pc = opt(pc)
	}

	e := &entry{
		key:   k,
		value: v,
	}

	kv.mx.Lock()
	defer kv.mx.Unlock()

	if pc.expiresAfter > 0 {
		e.timeout = new(timeout)
		e.expiresAfter = pc.expiresAfter
		e.isSliding = pc.isSliding
		e.expiresAt = time.Now().Add(pc.expiresAfter)
	}

	kv.kv[k] = e
}

//-----------------------------------------------------------------------------

// errors
var (
	ErrNotFound = errorf("NOT_FOUND")
	ErrCASCond  = errorf("CAS_COND_FAILED")
)

// CAS performs a compare and swap based on a vlue-condition
func (kv *store) CAS(k, v interface{}, cond func(interface{}, error) bool, options ...PutOption) error {
	kv.mx.Lock()
	defer kv.mx.Unlock()

	old, ok := kv.kv[k]
	var condErr error
	if !ok {
		condErr = ErrNotFound
	}
	var oldValue interface{}
	if old != nil {
		oldValue = old.value
	}
	if !cond(oldValue, condErr) {
		return ErrCASCond
	}

	var pc putConf
	for _, opt := range options {
		pc = opt(pc)
	}

	e := old
	if e == nil {
		e = &entry{
			key:   k,
			value: v,
		}
	}
	if pc.expiresAfter > 0 {
		e.timeout = new(timeout)
		e.expiresAfter = pc.expiresAfter
		e.isSliding = pc.isSliding
		e.expiresAt = time.Now().Add(pc.expiresAfter)
	}
	kv.kv[k] = e

	return nil
}

//-----------------------------------------------------------------------------

// Delete deletes an entry
func (kv *store) Delete(k interface{}) {
	kv.mx.Lock()
	defer kv.mx.Unlock()
	delete(kv.kv, k)
}

//-----------------------------------------------------------------------------

// Take .
func (kv *store) Take(k interface{}) (interface{}, bool) {
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

type timeout struct {
	expiresAt    time.Time // is not the exact time, but is the time window
	expiresAfter time.Duration
	isSliding    bool
}

func (to *timeout) slide() {
	if to == nil {
		return
	}
	if !to.isSliding {
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

type entry struct {
	*timeout
	key, value interface{}
}

//-----------------------------------------------------------------------------

// store is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type store struct {
	ctx                context.Context
	expirationInterval time.Duration
	onExpire           func(k, v interface{})

	mx sync.Mutex
	kv map[interface{}]*entry

	// timeWindows map[time.Time][]interface{} // PROBLEM: there will be gaps and there are some absent time windows'
	// use a sorted slice instead and we need GC with expirationInterval time.Duration anyway
	// and the context to stop the loop
}

//-----------------------------------------------------------------------------

func (kv *store) expireLoop() {
	var done <-chan struct{}
	if kv.ctx != nil {
		// context.Context Docs: Successive calls to Done return the same value.
		// so it's OK.
		done = kv.ctx.Done()
	}
	for {
		select {
		case <-done:
			return
		case <-time.After(kv.expirationInterval):
			kv.expireFunc()
		}
	}
}

// using type indexExpired []*entry ?
func (kv *store) expireFunc() {
	kv.mx.Lock()
	defer kv.mx.Unlock()

	expired := make(map[interface{}]interface{})
	for k, v := range kv.kv {
		if !v.expired() {
			continue
		}
		expired[k] = v.value
	}
	for k := range expired {
		delete(kv.kv, k)
	}
	if kv.onExpire == nil {
		return
	}
	go func() {
		for k, v := range expired {
			k, v := k, v
			supervisor.Supervise(func() error {
				kv.onExpire(k, v)
				return nil
			})
		}
	}()
}

//-----------------------------------------------------------------------------

// errString a string that inplements the error interface
type errString string

func (v errString) Error() string { return string(v) }

//-----------------------------------------------------------------------------
