package tinykv

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test01(t *testing.T) {
	rg := New(ExpirationInterval(time.Millisecond * 30))

	rg.Put(1, 1)
	v, ok := rg.Get(1)
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	rg.Put(2, 2, ExpiresAfter(time.Millisecond*50))
	v, ok = rg.Get(2)
	assert.True(t, ok)
	assert.Equal(t, 2, v)
	<-time.After(time.Millisecond * 100)

	v, ok = rg.Get(2)
	assert.False(t, ok)
	assert.NotEqual(t, 2, v)
}

func Test02(t *testing.T) {
	rg := New(ExpirationInterval(time.Millisecond * 30))

	rg.Put(1, 1)
	v, ok := rg.Get(1)
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	rg.Put(1, 1, ExpiresAfter(time.Millisecond*50), IsSliding(true))
	<-time.After(time.Millisecond * 40)
	v, ok = rg.Get(1)
	assert.True(t, ok)
	assert.Equal(t, 1, v)
	<-time.After(time.Millisecond * 10)
	v, ok = rg.Get(1)
	assert.True(t, ok)
	assert.Equal(t, 1, v)
	<-time.After(time.Millisecond * 10)
	v, ok = rg.Get(1)
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	<-time.After(time.Millisecond * 100)

	v, ok = rg.Get(1)
	assert.False(t, ok)
	assert.NotEqual(t, 1, v)
}

func Test03(t *testing.T) {
	var putAt time.Time
	var elapsed time.Duration
	kv := New(
		ExpirationInterval(time.Millisecond*50),
		OnExpire(func(k, v interface{}) {
			elapsed = time.Now().Sub(putAt)
		}))

	putAt = time.Now()
	kv.Put(1, 1, ExpiresAfter(time.Millisecond*10))

	<-time.After(time.Millisecond * 100)
	assert.WithinDuration(t, putAt, putAt.Add(elapsed), time.Millisecond*60)
}

// func BenchmarkGet01(b *testing.B) {
// 	rg := NewRegistry(nil, 0)
// 	for n := 0; n < b.N; n++ {
// 		rg.Get(1)
// 	}
// }

// func BenchmarkGet02(b *testing.B) {
// 	rg := NewRegistry(nil, 0)
// 	rg.Put(1, 1)
// 	for n := 0; n < b.N; n++ {
// 		rg.Get(1)
// 	}
// }

// func BenchmarkGet03(b *testing.B) {
// 	rg := NewRegistry(nil, 0)
// 	rg.PutWithExpiration(1, 1, time.Second)
// 	for n := 0; n < b.N; n++ {
// 		rg.Get(1)
// 	}
// }

// func BenchmarkPut01(b *testing.B) {
// 	rg := NewRegistry(nil, 0)
// 	for n := 0; n < b.N; n++ {
// 		rg.Put(1, 1)
// 	}
// }

// func BenchmarkCAS02(b *testing.B) {
// 	rg := NewRegistry(nil, 0)
// 	rg.Put(1, 1)
// 	for n := 0; n < b.N; n++ {
// 		rg.CAS(1, 2, func(interface{}) bool { return true })
// 	}
// }
