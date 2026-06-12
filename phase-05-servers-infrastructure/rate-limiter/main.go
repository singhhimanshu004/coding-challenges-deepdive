package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// buildLimiter constructs the chosen algorithm from CLI parameters. Keeping this
// in one place means the demo server, and any future caller, gets a Limiter
// without caring how it was built.
//
// For the windowed algorithms we interpret `rate` as "requests allowed per
// window" and `burst` (for the bucket algorithms) as the capacity. `window`
// controls the fixed/sliding window length.
func buildLimiter(algo string, rate, burst float64, window time.Duration, clock Clock) (Limiter, error) {
	switch algo {
	case "token-bucket":
		return NewTokenBucket(rate, burst, clock), nil
	case "leaky-bucket":
		return NewLeakyBucket(rate, burst, clock), nil
	case "sliding-window":
		return NewSlidingWindowLog(int(rate), window, clock), nil
	case "fixed-window":
		return NewFixedWindow(int(rate), window, clock), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q (want token-bucket | leaky-bucket | sliding-window | fixed-window)", algo)
	}
}

func main() {
	algo := flag.String("algo", "token-bucket", "algorithm: token-bucket | leaky-bucket | sliding-window | fixed-window")
	rate := flag.Float64("rate", 10, "allowed rate (tokens/sec for buckets; requests/window for windows)")
	burst := flag.Float64("burst", 20, "bucket capacity / max burst (bucket algorithms only)")
	window := flag.Duration("window", time.Second, "window length (window algorithms only), e.g. 1s, 1m")
	addr := flag.String("addr", ":8080", "address for the demo HTTP server to listen on")
	flag.Parse()

	limiter, err := buildLimiter(*algo, *rate, *burst, *window, realClock{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rate-limiter:", err)
		os.Exit(2)
	}

	// The protected handler — anything you'd normally serve goes here.
	demo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok — you are within the rate limit (%s)\n", *algo)
	})

	// Wrap it with our rate-limiting middleware. This is the whole point: the
	// business handler is untouched; limiting is a composable layer around it.
	mux := http.NewServeMux()
	mux.Handle("/", Middleware(limiter, clientIPKey)(demo))

	log.Printf("rate-limiter demo: algo=%s rate=%.2f burst=%.2f window=%s listening on %s",
		*algo, *rate, *burst, *window, *addr)
	log.Printf("try:  for i in $(seq 1 30); do curl -s -o /dev/null -w '%%{http_code}\\n' http://localhost%s/; done", *addr)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
