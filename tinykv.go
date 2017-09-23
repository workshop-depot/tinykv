package tinykv

import (
	"context"
	"sync"
	"time"

	"github.com/dc0d/supervisor"
)

// KV is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type KV interface {
	CAS(k, v interface{}, cond func(interface{}, error) bool, options ...PutOption) error
	Delete(k interface{})
	Get(k interface{}) (v interface{}, ok bool)
	Put(k, v interface{}, options ...PutOption)
	Take(k interface{}) (v interface{}, ok bool)
}

//-----------------------------------------------------------------------------

// store is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type store struct {
	ctx                context.Context
	expirationInterval time.Duration
	onExpire           func(k, v interface{})

	rwx          sync.RWMutex
	values       map[interface{}]interface{}
	expiresAt    map[interface{}]time.Time
	expiresAfter map[interface{}]time.Duration
	isSliding    map[interface{}]struct{}
}

//-----------------------------------------------------------------------------

// Option for store setup
type Option func(*store)

// Context sets the context
func Context(ctx context.Context) Option {
	return func(kv *store) { kv.ctx = ctx }
}

// OnExpire sets the onExpire func
func OnExpire(onExpire func(k, v interface{})) Option {
	return func(kv *store) { kv.onExpire = onExpire }
}

// ExpirationInterval sets the expirationInterval for its agent (goroutine)
func ExpirationInterval(expirationInterval time.Duration) Option {
	return func(kv *store) { kv.expirationInterval = expirationInterval }
}

//-----------------------------------------------------------------------------

// New creates a new *store
func New(options ...Option) KV {
	res := &store{
		values:       make(map[interface{}]interface{}),
		expiresAt:    make(map[interface{}]time.Time),
		expiresAfter: make(map[interface{}]time.Duration),
		isSliding:    make(map[interface{}]struct{}),
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

// Get .
func (kv *store) Get(k interface{}) (interface{}, bool) {
	slide := false
	kv.rwx.RLock()
	v, ok := kv.values[k]
	if ok {
		_, slide = kv.isSliding[k]
	}
	kv.rwx.RUnlock()
	if slide {
		kv.rwx.Lock()
		kv.expiresAt[k] = time.Now().Add(kv.expiresAfter[k])
		kv.rwx.Unlock()
	}
	return v, ok
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

	if pc.expiresAfter <= 0 {
		kv.rwx.Lock()
		kv.values[k] = v
		kv.rwx.Unlock()
		return
	}

	kv.rwx.Lock()
	kv.values[k] = v
	kv.expiresAfter[k] = pc.expiresAfter
	kv.expiresAt[k] = time.Now().Add(pc.expiresAfter)
	if pc.isSliding {
		kv.isSliding[k] = struct{}{}
	}
	kv.rwx.Unlock()
}

//-----------------------------------------------------------------------------

// errors
var (
	ErrNotFound = errorf("NOT_FOUND")
	ErrCASCond  = errorf("CAS_COND_FAILED")
)

// CAS performs a compare and swap based on a vlue-condition
func (kv *store) CAS(k, v interface{}, cond func(interface{}, error) bool, options ...PutOption) error {
	kv.rwx.Lock()
	defer kv.rwx.Unlock()
	old, ok := kv.values[k]
	var condErr error
	if !ok {
		condErr = ErrNotFound
	}
	if !cond(old, condErr) {
		return ErrCASCond
	}
	kv.values[k] = v

	if len(options) == 0 {
		return nil
	}

	var pc putConf
	for _, opt := range options {
		pc = opt(pc)
	}

	if pc.expiresAfter <= 0 {
		return nil
	}

	kv.expiresAfter[k] = pc.expiresAfter
	kv.expiresAt[k] = time.Now().Add(pc.expiresAfter)
	if pc.isSliding {
		kv.isSliding[k] = struct{}{}
	}

	return nil
}

//-----------------------------------------------------------------------------

// Delete deletes an entry
func (kv *store) Delete(k interface{}) {
	kv.rwx.Lock()
	kv.deleteEntry(k)
	kv.rwx.Unlock()
}

func (kv *store) deleteEntry(k interface{}) {
	delete(kv.expiresAt, k)
	delete(kv.expiresAfter, k)
	delete(kv.isSliding, k)
	delete(kv.values, k)
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

func (kv *store) expireFunc() {
	shouldExpire := kv.getThoseShouldExpire()
	if len(shouldExpire) == 0 {
		return
	}

	kv.rwx.Lock()
	for k := range shouldExpire {
		if !time.Now().After(kv.expiresAt[k]) {
			continue
		}
		kv.deleteEntry(k)
	}
	kv.rwx.Unlock()
	kv._notifyExpiration(shouldExpire)
}

func (kv *store) getThoseShouldExpire() (list map[interface{}]interface{}) {
	list = make(map[interface{}]interface{})
	kv.rwx.RLock()
	defer kv.rwx.RUnlock()
	for k, v := range kv.expiresAt {
		if !time.Now().After(v) {
			continue
		}
		list[k] = kv.values[k]
	}
	return
}

func (kv *store) _notifyExpiration(expired map[interface{}]interface{}) {
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

// Take .
func (kv *store) Take(k interface{}) (v interface{}, ok bool) {
	kv.rwx.Lock()
	defer kv.rwx.Unlock()
	v, ok = kv.values[k]
	if ok {
		kv.deleteEntry(k)
	}
	return v, ok
}

//-----------------------------------------------------------------------------
