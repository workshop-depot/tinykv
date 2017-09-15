package tinykv

import (
	"context"
	"sync"
	"time"

	"github.com/dc0d/supervisor"
)

//-----------------------------------------------------------------------------

// KV is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type KV struct {
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

// Option for KV setup
type Option func(*KV)

// Context sets the context
func Context(ctx context.Context) Option {
	return func(kv *KV) { kv.ctx = ctx }
}

// OnExpire sets the onExpire func
func OnExpire(onExpire func(k, v interface{})) Option {
	return func(kv *KV) { kv.onExpire = onExpire }
}

// ExpirationInterval sets the expirationInterval for its agent (goroutine)
func ExpirationInterval(expirationInterval time.Duration) Option {
	return func(kv *KV) { kv.expirationInterval = expirationInterval }
}

//-----------------------------------------------------------------------------

// New creates a new *KV
func New(options ...Option) *KV {
	res := &KV{
		values:       make(map[interface{}]interface{}),
		expiresAt:    make(map[interface{}]time.Time),
		expiresAfter: make(map[interface{}]time.Duration),
		isSliding:    make(map[interface{}]struct{}),
	}
	for _, opt := range options {
		opt(res)
	}
	if res.expirationInterval <= 0 {
		res.expirationInterval = 30
	}
	go res._expireLoop()
	return res
}

//-----------------------------------------------------------------------------

// Get .
func (kv *KV) Get(k interface{}) (interface{}, bool) {
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

// Put a (k, v) tuple in the KV store
func (kv *KV) Put(k, v interface{}, options ...PutOption) {
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
func (kv *KV) CAS(k, v interface{}, cond func(interface{}) bool) error {
	kv.rwx.Lock()
	defer kv.rwx.Unlock()
	old, ok := kv.values[k]
	if !ok {
		return ErrNotFound
	}
	if !cond(old) {
		return ErrCASCond
	}
	kv.values[k] = v
	return nil
}

//-----------------------------------------------------------------------------

// Delete deletes an entry
func (kv *KV) Delete(k interface{}) {
	kv.rwx.Lock()
	kv.deleteEntry(k)
	kv.rwx.Unlock()
}

func (kv *KV) deleteEntry(k interface{}) {
	delete(kv.expiresAt, k)
	delete(kv.expiresAfter, k)
	delete(kv.isSliding, k)
	delete(kv.values, k)
}

//-----------------------------------------------------------------------------

func (kv *KV) _expireLoop() {
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
			kv._expireFunc()
		}
	}
}

func (kv *KV) _expireFunc() {
	shouldExpire := kv._getThoseShouldExpire()
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

func (kv *KV) _getThoseShouldExpire() (list map[interface{}]interface{}) {
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

func (kv *KV) _notifyExpiration(expired map[interface{}]interface{}) {
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
