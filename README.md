# tinykv
tiny in-memory single-app kv (cache) with sliding expiration

# v1

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v1
```

# v2

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v2
```

# v3

* Using only `string` keys,
* Simplifying the interface,

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v3
```

Benchmarks:

```
$ go test -bench=.
BenchmarkGetNoValue-8          	20000000	        64.4 ns/op
BenchmarkGetValue-8            	20000000	        72.2 ns/op
BenchmarkGetSlidingTimeout-8   	20000000	        72.3 ns/op
BenchmarkPutOne-8              	10000000	       184 ns/op
BenchmarkPutN-8                	 2000000	       723 ns/op
BenchmarkPutExpire-8           	 5000000	       401 ns/op
BenchmarkCASTrue-8             	 5000000	       288 ns/op
BenchmarkCASFalse-8            	10000000	       277 ns/op
```