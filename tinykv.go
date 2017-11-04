package tinykv

import (
	"context"
	"sync"
	"time"

	"github.com/dc0d/retry"
)

//-----------------------------------------------------------------------------

type timeout struct {
	expiresAt    time.Time
	expiresAfter time.Duration
	isSliding    bool
}

func newTimeout(
	expiresAt time.Time,
	expiresAfter time.Duration,
	isSliding bool) *timeout {
	return &timeout{
		expiresAt:    expiresAt,
		expiresAfter: expiresAfter,
		isSliding:    isSliding,
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
}

//-----------------------------------------------------------------------------

// store is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type store struct {
	ctx                context.Context
	expirationInterval time.Duration
	onExpire           func(k string, v interface{})

	mx sync.Mutex
	kv map[string]*entry
}

// New creates a new *store
func New(options ...Option) KV {
	res := &store{
		kv: make(map[string]*entry),
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
	if opt.expiresAfter > 0 {
		to := new(timeout)
		to.expiresAfter = opt.expiresAfter
		to.isSliding = opt.isSliding
		to.expiresAt = time.Now().Add(to.expiresAfter)
		e.timeout = to
	}
	kv.mx.Lock()
	defer kv.mx.Unlock()
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

// Option for store setup
type Option func(*store)

// OnExpire sets the onExpire func
func OnExpire(onExpire func(k string, v interface{})) Option {
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

	expired := make(map[string]interface{})
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
			retry.Try(func() error {
				kv.onExpire(k, v)
				return nil
			})
		}
	}()
}

//-----------------------------------------------------------------------------
