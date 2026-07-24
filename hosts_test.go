package main

import (
	"strings"
	"testing"
)

func TestHostsLine(t *testing.T) {
	got := hostsLine("wififun.net", "127.0.0.1")
	if !strings.Contains(got, "127.0.0.1") || !strings.Contains(got, "wififun.net") {
		t.Fatalf("hostsLine=%q", got)
	}
	if !strings.Contains(got, hostsMarker) {
		t.Fatal("should carry a removable marker comment")
	}
}
