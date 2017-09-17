# tinykv
tiny in-memory single-app kv (cache) with sliding expiration

# v1

```
$ go test -bench=.
BenchmarkGetNoValue-8          	30000000	        53.5 ns/op
BenchmarkGetValue-8            	20000000	        91.9 ns/op
BenchmarkGetSlidingTimeout-8   	20000000	        90.1 ns/op
BenchmarkPut-8                 	10000000	       238 ns/op
BenchmarkCASTrue-8             	 5000000	       389 ns/op
BenchmarkCASFalse-8            	 5000000	       302 ns/op
```