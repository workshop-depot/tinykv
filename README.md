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

Benchmarks:

```
$ go test -bench=.
BenchmarkGetNoValue-8          	20000000	        64.6 ns/op
BenchmarkGetValue-8            	20000000	        93.8 ns/op
BenchmarkGetSlidingTimeout-8   	20000000	        95.8 ns/op
BenchmarkPutOne-8              	10000000	        176 ns/op
BenchmarkPutN-8                	 2000000	        729 ns/op
BenchmarkPutExpire-8           	 5000000	        403 ns/op
BenchmarkCASTrue-8             	10000000	        135 ns/op
BenchmarkCASFalse-8            	20000000	        93.6 ns/op
```