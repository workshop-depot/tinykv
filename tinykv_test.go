package tinykv

import (
	"fmt"
	"math/rand"
	"sync/atomic"
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

func Test04(t *testing.T) {
	kv := New(
		ExpirationInterval(time.Millisecond*10),
		OnExpire(func(k, v interface{}) {
			t.Fatal(k, v)
		}))

	kv.Put(1, 1, ExpiresAfter(time.Millisecond*10000))
	<-time.After(time.Millisecond * 50)
	kv.Delete(1)
	kv.Delete(1)

	<-time.After(time.Millisecond * 100)
	_, ok := kv.Get(1)
	assert.False(t, ok)
}

func Test05(t *testing.T) {
	N := 10000
	var cnt int64
	kv := New(
		ExpirationInterval(time.Millisecond*10),
		OnExpire(func(k, v interface{}) {
			atomic.AddInt64(&cnt, 1)
		}))

	src := rand.NewSource(time.Now().Unix())
	rnd := rand.New(src)
	for i := 0; i < N; i++ {
		k := i
		kv.Put(k, fmt.Sprintf("VAL::%v", k),
			ExpiresAfter(
				time.Millisecond*time.Duration(rnd.Intn(10)+1)))
	}

	<-time.After(time.Millisecond * 100)
	for i := 0; i < N; i++ {
		k := i
		_, ok := kv.Get(k)
		assert.False(t, ok)
	}
}

func Test06(t *testing.T) {
	kv := New(
		ExpirationInterval(time.Millisecond),
		OnExpire(func(k, v interface{}) {
			t.Fail()
		}))

	kv.Put(1, 1, ExpiresAfter(10*time.Millisecond), IsSliding(true))

	for i := 0; i < 100; i++ {
		_, ok := kv.Get(1)
		assert.True(t, ok)
		<-time.After(time.Millisecond)
	}
	kv.Delete(1)

	<-time.After(time.Millisecond * 30)

	_, ok := kv.Get(1)
	assert.False(t, ok)
}

func Test07(t *testing.T) {
	assert := assert.New(t)

	kv := New()
	kv.Put(1, 1)
	v, ok := kv.Take(1)
	assert.True(ok)
	assert.Equal(1, v)

	_, ok = kv.Get(1)
	assert.False(ok)
}

func Test08(t *testing.T) {
	assert := assert.New(t)

	kv := New()
	err := kv.CAS(
		"QQG", "G",
		func(interface{}, error) bool { return true },
		ExpiresAfter(time.Millisecond))
	assert.NoError(err)

	v, ok := kv.Take("QQG")
	assert.True(ok)
	assert.Equal("G", v)
}

func Test09(t *testing.T) {
	assert := assert.New(t)

	e1 := errorf("ERR")
	e2 := errorf("ERR")
	assert.Equal(e1, e2)

	e2 = errorf("ERR 2")
	assert.NotEqual(e1, e2)
}

func Test10(t *testing.T) {
	assert := assert.New(t)

	key := "QQG"

	kv := New(ExpirationInterval(time.Millisecond))
	err := kv.CAS(
		key, "G",
		func(interface{}, error) bool { return true },
		ExpiresAfter(time.Millisecond*30))
	assert.NoError(err)

	v, ok := kv.Get(key)
	assert.True(ok)
	assert.Equal("G", v)

	<-time.After(time.Millisecond * 20)

	err = kv.CAS(key, "OK", func(currentValue interface{}, err error) bool {
		assert.NoError(err)
		assert.Equal("G", currentValue)
		return true
	})
	assert.NoError(err)

	<-time.After(time.Millisecond * 11)
	_, ok = kv.Get(key)
	assert.False(ok)
}

func Test11(t *testing.T) {
	assert := assert.New(t)

	key := "QQG"

	kv := New(ExpirationInterval(time.Millisecond))
	err := kv.CAS(
		key, "G",
		func(interface{}, error) bool { return true },
		IsSliding(true),
		ExpiresAfter(time.Millisecond*15))
	assert.NoError(err)

	<-time.After(time.Millisecond * 12)

	v, ok := kv.Get(key)
	assert.True(ok)
	assert.Equal("G", v)

	<-time.After(time.Millisecond * 12)

	err = kv.CAS(key, "OK", func(currentValue interface{}, err error) bool {
		assert.NoError(err)
		assert.Equal("G", currentValue)
		return true
	})
	assert.NoError(err)

	<-time.After(time.Millisecond * 12)

	_, ok = kv.Get(key)
	assert.True(ok)
}

func BenchmarkGetNoValue(b *testing.B) {
	rg := New()
	for n := 0; n < b.N; n++ {
		rg.Get(1)
	}
}

func BenchmarkGetValue(b *testing.B) {
	rg := New()
	rg.Put(1, 1)
	for n := 0; n < b.N; n++ {
		rg.Get(1)
	}
}

func BenchmarkGetSlidingTimeout(b *testing.B) {
	rg := New()
	rg.Put(1, 1, ExpiresAfter(time.Second*10))
	for n := 0; n < b.N; n++ {
		rg.Get(1)
	}
}

func BenchmarkPutOne(b *testing.B) {
	rg := New()
	for n := 0; n < b.N; n++ {
		rg.Put(1, 1)
	}
}

func BenchmarkPutN(b *testing.B) {
	rg := New()
	for n := 0; n < b.N; n++ {
		rg.Put(n, n)
	}
}

func BenchmarkPutExpire(b *testing.B) {
	rg := New()
	for n := 0; n < b.N; n++ {
		rg.Put(1, 1, ExpiresAfter(time.Second*10))
	}
}

func BenchmarkCASTrue(b *testing.B) {
	rg := New()
	rg.Put(1, 1)
	for n := 0; n < b.N; n++ {
		rg.CAS(1, 2, func(interface{}, error) bool { return true })
	}
}

func BenchmarkCASFalse(b *testing.B) {
	rg := New()
	rg.Put(1, 1)
	for n := 0; n < b.N; n++ {
		rg.CAS(1, 2, func(interface{}, error) bool { return false })
	}
}
