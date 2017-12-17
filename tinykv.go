package tinykv

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"
)

//-----------------------------------------------------------------------------

type timeout struct {
	expiresAt    time.Time
	expiresAfter time.Duration
	isSliding    bool
}

func newTimeout(
	expiresAfter time.Duration,
	isSliding bool) *timeout {
	to := new(timeout)
	to.expiresAfter = expiresAfter
	to.isSliding = isSliding
	to.expiresAt = time.Now().Add(to.expiresAfter)
	return to
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
	Stop()
}

//-----------------------------------------------------------------------------

// store is a registry for values (like/is a concurrent map) with timeout and sliding timeout
type store struct {
	ctx                context.Context
	cancel             context.CancelFunc
	expirationInterval time.Duration
	onExpire           func(k string, v interface{})

	mx sync.Mutex
	kv map[string]*entry
}

// New creates a new *store
func New(expirationInterval time.Duration,
	onExpire ...func(k string, v interface{})) KV {
	res := &store{
		kv:                 make(map[string]*entry),
		expirationInterval: expirationInterval,
	}
	if len(onExpire) > 0 && onExpire[0] != nil {
		res.onExpire = onExpire[0]
	}
	if res.expirationInterval <= 0 {
		res.expirationInterval = 30 * time.Second
	}
	res.ctx, res.cancel = context.WithCancel(context.Background())
	go res.expireLoop()
	return res
}

// Stop stops the goroutine
func (kv *store) Stop() {
	kv.cancel()
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
	if opt.expiresAfter > 0 {
		e.timeout = newTimeout(opt.expiresAfter, opt.isSliding)
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

func (kv *store) expireLoop() {
	for {
		select {
		case <-kv.ctx.Done():
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
	go notifyExpirations(expired, kv.onExpire)
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

// constants
const (
	DefaultTimeout = time.Minute * 3
)

// errors
var (
	ErrCASCond = errors.Errorf("CAS COND FAILED")
)

//-----------------------------------------------------------------------------
