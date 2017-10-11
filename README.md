# tinykv
tiny in-memory single-app kv (cache) with sliding expiration

# v1

Get it using:

```bash
$ go get gopkg.in/dc0d/tinykv.v1
```

Benchmarks:

```
$ go test -bench=.
BenchmarkGetNoValue-8          	30000000	        53.0 ns/op
BenchmarkGetValue-8            	20000000	        91.3 ns/op
BenchmarkGetSlidingTimeout-8   	20000000	        91.4 ns/op
BenchmarkPutOne-8              	10000000	       210 ns/op
BenchmarkPutN-8                	 2000000	       851 ns/op
BenchmarkPutExpire-8           	 3000000	       530 ns/op
BenchmarkCASTrue-8             	 5000000	       360 ns/op
BenchmarkCASFalse-8            	 5000000	       262 ns/op
```