package minecraft

import "testing"

func TestParseProcStatTicks(t *testing.T) {
	tests := []struct {
		name string
		stat string
		want uint64
		ok   bool
	}{
		{"plain command", "1234 (java) S 1 1234 1234 0 -1 4194304 10 0 2 0 250 50 0 0 20 0 3 0 100 123456 789", 300, true},
		{"command with spaces and parentheses", "1234 (my ) proc) S 1 1234 1234 0 -1 4194304 10 0 2 0 7 3 0 0 20 0 3 0 100 123456 789", 10, true},
		{"truncated", "1234 (java) S 1 1234", 0, false},
		{"no command", "garbage", 0, false},
		{"non numeric ticks", "1234 (java) S 1 1234 1234 0 -1 4194304 10 0 2 0 x 50 0", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseProcStatTicks(tt.stat)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("parseProcStatTicks = %d, %v; want %d, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseStatmRSSBytes(t *testing.T) {
	got, ok := parseStatmRSSBytes("5000 678 90 1 0 400 0", 4096)
	if !ok || got != 678*4096 {
		t.Fatalf("parseStatmRSSBytes = %d, %v; want %d, true", got, ok, 678*4096)
	}
	if _, ok := parseStatmRSSBytes("5000", 4096); ok {
		t.Fatalf("expected a single-field statm to be rejected")
	}
}

func TestParseMemTotalBytes(t *testing.T) {
	meminfo := "MemFree:         1000 kB\nMemTotal:       16384000 kB\nMemAvailable:    2000 kB\n"
	if got := parseMemTotalBytes(meminfo); got != 16384000*1024 {
		t.Fatalf("parseMemTotalBytes = %d; want %d", got, 16384000*1024)
	}
	if got := parseMemTotalBytes("MemFree: 1 kB\n"); got != 0 {
		t.Fatalf("expected 0 without MemTotal, got %d", got)
	}
}
