// Command crontab parses, explains, and computes upcoming run times for cron
// expressions. It focuses on the interesting part of cron — expression parsing
// and schedule math — not on actually daemonizing and running jobs.
package main

import (
	"os"
)

// main is intentionally tiny: all real work lives in run() so it can be tested
// against in-memory buffers. main only wires up the real OS streams and exits.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
