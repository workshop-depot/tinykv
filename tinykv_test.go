package tinykv

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var _ KV = &store{}

func Test01(t *testing.T) {
	assert := assert.New(t)
	rg := New(time.Millisecond * 30)

	rg.Put("1", 1)
	v, ok := rg.Get("1")
	assert.True(ok)
	assert.Equal(1, v)

	rg.Put("2", 2, ExpiresAfter(time.Millisecond*50))
	v, ok = rg.Get("2")
	assert.True(ok)
	assert.Equal(2, v)
	<-time.After(time.Millisecond * 100)

	v, ok = rg.Get("2")
	assert.False(ok)
	assert.NotEqual(2, v)
}

func Test02(t *testing.T) {
	assert := assert.New(t)
	rg := New(time.Millisecond * 30)

	rg.Put("1", 1)
	v, ok := rg.Get("1")
	assert.True(ok)
	assert.Equal(1, v)

	rg.Put("1", 1, ExpiresAfter(time.Millisecond*50), IsSliding(true))
	<-time.After(time.Millisecond * 40)
	v, ok = rg.Get("1")
	assert.True(ok)
	assert.Equal(1, v)
	<-time.After(time.Millisecond * 10)
	v, ok = rg.Get("1")
	assert.True(ok)
	assert.Equal(1, v)
	<-time.After(time.Millisecond * 10)
	v, ok = rg.Get("1")
	assert.True(ok)
	assert.Equal(1, v)

	<-time.After(time.Millisecond * 100)

	v, ok = rg.Get("1")
	assert.False(ok)
	assert.NotEqual(1, v)
}

func Test03(t *testing.T) {
	assert := assert.New(t)
	var putAt time.Time
	var elapsed time.Duration
	kv := New(
		time.Millisecond*50,
		func(k string, v interface{}) {
			elapsed = time.Now().Sub(putAt)
		})

	putAt = time.Now()
	kv.Put("1", 1, ExpiresAfter(time.Millisecond*10))

	<-time.After(time.Millisecond * 100)
	assert.WithinDuration(putAt, putAt.Add(elapsed), time.Millisecond*60)
}

func Test04(t *testing.T) {
	assert := assert.New(t)
	kv := New(
		time.Millisecond*10,
		func(k string, v interface{}) {
			t.Fatal(k, v)
		})

	err := kv.Put("1", 1, ExpiresAfter(time.Millisecond*10000))
	assert.NoError(err)
	<-time.After(time.Millisecond * 50)
	kv.Delete("1")
	kv.Delete("1")

	<-time.After(time.Millisecond * 100)
	_, ok := kv.Get("1")
	assert.False(ok)
}

func Test05(t *testing.T) {
	assert := assert.New(t)
	N := 10000
	var cnt int64
	kv := New(
		time.Millisecond*10,
		func(k string, v interface{}) {
			atomic.AddInt64(&cnt, 1)
		})

	src := rand.NewSource(time.Now().Unix())
	rnd := rand.New(src)
	for i := 0; i < N; i++ {
		k := fmt.Sprintf("%d", i)
		kv.Put(k, fmt.Sprintf("VAL::%v", k),
			ExpiresAfter(
				time.Millisecond*time.Duration(rnd.Intn(10)+1)))
	}

	<-time.After(time.Millisecond * 100)
	for i := 0; i < N; i++ {
		k := fmt.Sprintf("%d", i)
		_, ok := kv.Get(k)
		assert.False(ok)
	}
}

func Test06(t *testing.T) {
	assert := assert.New(t)
	kv := New(
		time.Millisecond,
		func(k string, v interface{}) {
			t.Fail()
		})

	err := kv.Put("1", 1, ExpiresAfter(10*time.Millisecond), IsSliding(true))
	assert.NoError(err)

	for i := 0; i < 100; i++ {
		_, ok := kv.Get("1")
		assert.True(ok)
		<-time.After(time.Millisecond)
	}
	kv.Delete("1")

	<-time.After(time.Millisecond * 30)

	_, ok := kv.Get("1")
	assert.False(ok)
}

func Test07(t *testing.T) {
	assert := assert.New(t)

	kv := New(-1)
	kv.Put("1", 1)
	v, ok := kv.Take("1")
	assert.True(ok)
	assert.Equal(1, v)

	_, ok = kv.Get("1")
	assert.False(ok)
}

func Test08(t *testing.T) {
	assert := assert.New(t)

	kv := New(-1)
	err := kv.Put(
		"QQG", "G",
		CAS(func(interface{}, bool) bool { return true }),
		ExpiresAfter(time.Millisecond))
	assert.NoError(err)

	v, ok := kv.Take("QQG")
	assert.True(ok)
	assert.Equal("G", v)
}

// ignore new timeouts when cas, and just use the old ones from the old value (if exists)
func Test09IgnoreTimeoutParamsOnCAS(t *testing.T) {
	assert := assert.New(t)

	key := "QQG"

	kv := New(time.Millisecond)
	err := kv.Put(
		key, "G",
		CAS(func(interface{}, bool) bool { return true }),
		ExpiresAfter(time.Millisecond*30))
	assert.NoError(err)

	v, ok := kv.Get(key)
	assert.True(ok)
	assert.Equal("G", v)

	<-time.After(time.Millisecond * 20)

	err = kv.Put(key, "OK",
		CAS(func(currentValue interface{}, found bool) bool {
			assert.True(found)
			assert.Equal("G", currentValue)
			return true
		}))
	assert.NoError(err)

	<-time.After(time.Millisecond * 12)
	_, ok = kv.Get(key)
	assert.False(ok)
}

func Test10(t *testing.T) {
	assert := assert.New(t)

	key := "QQG"

	kv := New(time.Millisecond)
	err := kv.Put(
		key, "G",
		CAS(func(interface{}, bool) bool { return true }),
		IsSliding(true),
		ExpiresAfter(time.Millisecond*15))
	assert.NoError(err)

	<-time.After(time.Millisecond * 12)

	v, ok := kv.Get(key)
	assert.True(ok)
	assert.Equal("G", v)

	<-time.After(time.Millisecond * 12)

	err = kv.Put(key, "OK",
		CAS(func(currentValue interface{}, found bool) bool {
			assert.True(found)
			assert.Equal("G", currentValue)
			return true
		}))
	assert.NoError(err)

	<-time.After(time.Millisecond * 12)

	_, ok = kv.Get(key)
	assert.True(ok)
}

func BenchmarkGetNoValue(b *testing.B) {
	rg := New(-1)
	for n := 0; n < b.N; n++ {
		rg.Get("1")
	}
}

func BenchmarkGetValue(b *testing.B) {
	rg := New(-1)
	rg.Put("1", 1)
	for n := 0; n < b.N; n++ {
		rg.Get("1")
	}
}

func BenchmarkGetSlidingTimeout(b *testing.B) {
	rg := New(-1)
	rg.Put("1", 1, ExpiresAfter(time.Second*10))
	for n := 0; n < b.N; n++ {
		rg.Get("1")
	}
}

func BenchmarkPutOne(b *testing.B) {
	rg := New(-1)
	for n := 0; n < b.N; n++ {
		rg.Put("1", 1)
	}
}

func BenchmarkPutN(b *testing.B) {
	rg := New(-1)
	for n := 0; n < b.N; n++ {
		k := strconv.Itoa(n)
		rg.Put(k, n)
	}
}

func BenchmarkPutExpire(b *testing.B) {
	rg := New(-1)
	for n := 0; n < b.N; n++ {
		rg.Put("1", 1, ExpiresAfter(time.Second*10))
	}
}

func BenchmarkCASTrue(b *testing.B) {
	rg := New(-1)
	rg.Put("1", 1)
	for n := 0; n < b.N; n++ {
		rg.Put("1", 2, CAS(func(interface{}, bool) bool { return true }))
	}
}

func BenchmarkCASFalse(b *testing.B) {
	rg := New(-1)
	rg.Put("1", 1)
	for n := 0; n < b.N; n++ {
		rg.Put("1", 2, CAS(func(interface{}, bool) bool { return false }))
	}
}
