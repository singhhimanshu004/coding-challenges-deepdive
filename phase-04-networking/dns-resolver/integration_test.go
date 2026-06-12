package main

// Network integration test. It is GUARDED so it never fails offline or in CI:
// it only runs when DNS_NETWORK_TEST=1 is set in the environment. This keeps
// the default `go test ./...` hermetic while still letting you exercise the
// full UDP round-trip against a real resolver on demand:
//
//	DNS_NETWORK_TEST=1 CGO_ENABLED=0 go test -run Network -v ./...

import (
	"net"
	"os"
	"testing"
)

func TestNetworkResolveA(t *testing.T) {
	if os.Getenv("DNS_NETWORK_TEST") != "1" {
		t.Skip("set DNS_NETWORK_TEST=1 to run the live-network DNS test")
	}

	resp, err := resolveRecursive("8.8.8.8:53", "example.com", TypeA)
	if err != nil {
		t.Fatalf("resolveRecursive: %v", err)
	}
	found := false
	for _, rr := range resp.Answers {
		if rr.Type == TypeA {
			if net.ParseIP(rr.Data) == nil {
				t.Errorf("answer %q is not a valid IP", rr.Data)
			}
			found = true
		}
	}
	if !found {
		t.Error("no A record returned for example.com")
	}
}

func TestNetworkResolveIterative(t *testing.T) {
	if os.Getenv("DNS_NETWORK_TEST") != "1" {
		t.Skip("set DNS_NETWORK_TEST=1 to run the live-network DNS test")
	}

	resp, err := resolveIterative("example.com", TypeA, nil)
	if err != nil {
		t.Fatalf("resolveIterative: %v", err)
	}
	if !hasType(resp.Answers, TypeA) {
		t.Error("iterative walk returned no A record")
	}
}
