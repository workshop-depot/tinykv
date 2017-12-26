[![GoDoc](https://godoc.org/github.com/dc0d/tinykv?status.svg)](https://godoc.org/github.com/dc0d/tinykv) [![Go Report Card](https://goreportcard.com/badge/github.com/dc0d/tinykv)](https://goreportcard.com/report/github.com/dc0d/tinykv)
<br/>

# tinykv
tiny in-memory single-app kv (cache) with explicit and sliding expiration


# v4

* Heap-based expiration strategy

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v4
```

# v3

* Using only `string` keys,
* Simplifying the interface,

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v3
```

# v2

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v2
```

# v1

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v1
```

Benchmarks:

```
$ go test -bench=.
BenchmarkGetNoValue-8          	20000000	        72.6 ns/op
BenchmarkGetValue-8            	20000000	        78.3 ns/op
BenchmarkGetSlidingTimeout-8   	10000000	       128 ns/op
BenchmarkPutOne-8              	10000000	       219 ns/op
BenchmarkPutN-8                	 2000000	       814 ns/op
BenchmarkPutExpire-8           	 2000000	       882 ns/op
BenchmarkCASTrue-8             	 5000000	       268 ns/op
BenchmarkCASFalse-8            	10000000	       260 ns/op
```