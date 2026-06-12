package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    []int
		wantErr bool
	}{
		{"single", "8080", []int{8080}, false},
		{"list", "22,80,443", []int{22, 80, 443}, false},
		{"range", "20-25", []int{20, 21, 22, 23, 24, 25}, false},
		{"mixed", "80,20-22,443", []int{20, 21, 22, 80, 443}, false},
		{"dedup_and_sort", "443,80,80,22", []int{22, 80, 443}, false},
		{"reversed_range", "25-20", []int{20, 21, 22, 23, 24, 25}, false},
		{"whitespace", " 22 , 80 ", []int{22, 80}, false},
		{"zero_invalid", "0", nil, true},
		{"too_high", "70000", nil, true},
		{"range_out_of_bounds", "1-70000", nil, true},
		{"garbage", "abc", nil, true},
		{"empty", "", nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePorts(tc.spec)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePorts(%q) expected error, got %v", tc.spec, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePorts(%q) unexpected error: %v", tc.spec, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsePorts(%q) = %v, want %v", tc.spec, got, tc.want)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantHost    string
		wantWorkers int
		wantTimeout time.Duration
		wantNPorts  int
		wantErr     bool
	}{
		{
			name:        "host_only_defaults",
			args:        []string{"example.com"},
			wantHost:    "example.com",
			wantWorkers: 100,
			wantTimeout: time.Second,
			wantNPorts:  1024,
		},
		{
			name:        "all_flags",
			args:        []string{"127.0.0.1", "--ports", "22,80,443", "--workers", "50", "--timeout", "500ms"},
			wantHost:    "127.0.0.1",
			wantWorkers: 50,
			wantTimeout: 500 * time.Millisecond,
			wantNPorts:  3,
		},
		{
			name:        "short_flags",
			args:        []string{"-p", "1-10", "-w", "5", "-t", "2s", "localhost"},
			wantHost:    "localhost",
			wantWorkers: 5,
			wantTimeout: 2 * time.Second,
			wantNPorts:  10,
		},
		{"missing_host", []string{"--ports", "80"}, "", 0, 0, 0, true},
		{"bad_workers", []string{"host", "--workers", "0"}, "", 0, 0, 0, true},
		{"bad_timeout", []string{"host", "--timeout", "nope"}, "", 0, 0, 0, true},
		{"unknown_flag", []string{"host", "--bogus"}, "", 0, 0, 0, true},
		{"flag_without_value", []string{"host", "--ports"}, "", 0, 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := parseArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseArgs(%v) expected error", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%v) unexpected error: %v", tc.args, err)
			}
			if opt.host != tc.wantHost {
				t.Errorf("host = %q, want %q", opt.host, tc.wantHost)
			}
			if opt.workers != tc.wantWorkers {
				t.Errorf("workers = %d, want %d", opt.workers, tc.wantWorkers)
			}
			if opt.timeout != tc.wantTimeout {
				t.Errorf("timeout = %s, want %s", opt.timeout, tc.wantTimeout)
			}
			if len(opt.ports) != tc.wantNPorts {
				t.Errorf("len(ports) = %d, want %d", len(opt.ports), tc.wantNPorts)
			}
		})
	}
}
