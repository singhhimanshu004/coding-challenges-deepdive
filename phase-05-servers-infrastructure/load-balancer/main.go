// Command load-balancer is an HTTP reverse-proxy load balancer. It listens on a
// single address and forwards each incoming request to one of several backend
// origin servers, returning the backend's response to the client.
//
// It is the natural sequel to Challenge 30 (web-server): point this in front of
// two or more of those servers and it spreads traffic across them, skips ones
// that fall over, and brings them back when they recover. Compared with the
// Phase 4 forward proxy (Challenge 29), this is a REVERSE proxy: clients address
// the balancer as if it were the server and never know which backend served
// them, whereas a forward proxy sits next to the client and reaches arbitrary
// origins.
//
// We use net/http + httputil.ReverseProxy for the proxying mechanics on purpose.
// The lesson here is the logic AROUND the proxy — the scheduling algorithms and
// the health checks — which are all hand-written.
//
// Usage:
//
//	load-balancer [--addr :8080] [--backends url1,url2,...] \
//	              [--algo round-robin|least-conn|random|weighted] \
//	              [--health-interval 5s] [--health-path /health] \
//	              [--health-timeout 2s] [--verbose]
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "address the load balancer listens on (host:port)")
	backendsCSV := flag.String("backends", "", "comma-separated backend URLs, e.g. http://127.0.0.1:9001,http://127.0.0.1:9002")
	algo := flag.String("algo", "round-robin", "scheduling algorithm: round-robin | least-conn | random | weighted")
	healthInterval := flag.Duration("health-interval", 5*time.Second, "how often to actively probe each backend")
	healthPath := flag.String("health-path", "/health", "path to GET when probing backend health")
	healthTimeout := flag.Duration("health-timeout", 2*time.Second, "per-probe timeout")
	verbose := flag.Bool("verbose", false, "log every routing decision")
	flag.Parse()

	logger := log.New(os.Stderr, "load-balancer: ", log.LstdFlags)

	backends, err := parseBackends(*backendsCSV, logger)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	if len(backends) == 0 {
		logger.Fatalf("no backends given; pass --backends url1,url2,...")
	}

	scheduler, err := newScheduler(*algo)
	if err != nil {
		logger.Fatalf("%v", err)
	}

	pool := NewPool(backends)

	// Active health checking runs in the background for the life of the process.
	// A cancellable context lets us stop the probe goroutine on shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc := NewHealthChecker(pool, *healthInterval, *healthTimeout, *healthPath, logger)
	hc.Start(ctx)

	// Quiet the routing logger unless --verbose.
	routeLogger := logger
	if !*verbose {
		routeLogger = log.New(io.Discard, "", 0)
	}
	lb := NewLoadBalancer(pool, scheduler, routeLogger)

	logger.Printf("balancing %d backend(s) on %s using %s scheduling (health %s every %s)",
		len(backends), *addr, scheduler.Name(), *healthPath, *healthInterval)

	srv := &http.Server{Addr: *addr, Handler: lb}

	// Graceful shutdown on Ctrl-C / SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Printf("shutting down...")
		cancel()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("%v", err)
	}
}

// parseBackends turns a comma-separated URL list into Backend values. Each URL
// must be absolute (have a scheme and host) so the reverse proxy knows where to
// send traffic. A backend may carry an optional "#weight" suffix for weighted
// scheduling, e.g. http://10.0.0.1:80#3.
func parseBackends(csv string, logger *log.Logger) ([]*Backend, error) {
	markDown := func(b *Backend) {
		logger.Printf("backend %s marked DOWN (transport error)", b.URL)
	}

	var backends []*Backend
	for _, raw := range strings.Split(csv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		weight := 1
		if i := strings.LastIndex(raw, "#"); i >= 0 {
			if w, perr := parseWeight(raw[i+1:]); perr == nil {
				weight = w
				raw = raw[:i]
			}
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, &backendError{raw: raw}
		}
		backends = append(backends, newBackend(u, weight, markDown))
	}
	return backends, nil
}

type backendError struct{ raw string }

func (e *backendError) Error() string {
	return "invalid backend URL \"" + e.raw + "\" (want absolute, e.g. http://host:port)"
}

func parseWeight(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, &backendError{raw: s}
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 {
		n = 1
	}
	return n, nil
}
