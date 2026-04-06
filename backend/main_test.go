package main

import "testing"

func TestHostsFileContainsHostname(t *testing.T) {
	content := []byte(`
127.0.0.1 localhost
127.0.1.1 orexa-panel orexa-panel.local
# 10.0.0.1 ignored-host
`)

	if !hostsFileContainsHostname(content, "orexa-panel") {
		t.Fatalf("expected hostname to be detected in hosts content")
	}
	if !hostsFileContainsHostname(content, "OREXA-PANEL.LOCAL") {
		t.Fatalf("expected case-insensitive alias match")
	}
	if hostsFileContainsHostname(content, "missing-host") {
		t.Fatalf("did not expect missing host to match")
	}
}
