package minecraft

import "testing"

func TestNeoForgeVersionMapping(t *testing.T) {
	tests := []struct{ neoforge, mc, prefix string }{
		{"21.1.80", "1.21.1", "21.1."},
		{"20.4.237", "1.20.4", "20.4."},
		{"26.1.0.19", "26.1", "26.1.0."},
		{"26.1.1.15", "26.1.1", "26.1.1."},
		{"26.1.2.126", "26.1.2", "26.1.2."},
		{"26.3.0.101", "26.3", "26.3.0."},
	}
	for _, tt := range tests {
		t.Run(tt.neoforge, func(t *testing.T) {
			if mc, ok := neoForgeMCVersion(tt.neoforge); !ok || mc != tt.mc {
				t.Fatalf("neoForgeMCVersion(%q) = %q, %v; want %q", tt.neoforge, mc, ok, tt.mc)
			}
			if prefix, ok := neoForgePrefix(tt.mc); !ok || prefix != tt.prefix {
				t.Fatalf("neoForgePrefix(%q) = %q, %v; want %q", tt.mc, prefix, ok, tt.prefix)
			}
		})
	}
	for _, bad := range []string{"", "21", "26.1", "26.x.0.1", "garbage"} {
		if mc, ok := neoForgeMCVersion(bad); ok {
			t.Fatalf("neoForgeMCVersion(%q) = %q; want rejection", bad, mc)
		}
	}
	for _, bad := range []string{"", "1.21", "26", "26.x", "garbage"} {
		if p, ok := neoForgePrefix(bad); ok {
			t.Fatalf("neoForgePrefix(%q) = %q; want rejection", bad, p)
		}
	}
}
