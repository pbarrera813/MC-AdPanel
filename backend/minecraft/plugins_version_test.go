package minecraft

import "testing"

func TestCompareLatestToCurrent(t *testing.T) {
	tests := []struct {
		current, latest string
		want            int
		confident       bool
	}{
		{"1.0.0", "1.0.0", 0, true},
		{"1.0.0", "1.0.1", 1, true},
		{"1.2.0", "1.1.9", -1, true},
		{"v1.2.3", "1.2.3", 0, true},
		{"1.2.3", "v1.2.4", 1, true},
		{"1.2.3", "1.2.3-SNAPSHOT", -1, true},
		{"1.2.3-SNAPSHOT", "1.2.3", 1, true},
		{"2.0-beta", "2.0", 1, true},
		{"1.0", "1.0.1", 1, true},
		{"5.4.0-SNAPSHOT", "5.4.0", 1, true},
		{"5.4.0-rc1", "5.4.0-rc2", 1, true},
		{"mc1.21.1-0.15.2", "mc1.21.1-0.15.3", 1, true},
		{"mc1.21.1-0.15.2", "mc1.21.1-0.15.2", 0, true},
		{"fabric-1.21.1+0.15.2", "fabric-1.21.1+0.16.0", 1, true},
		{"1.21.1-0.15.2", "1.21.1-0.15.2", 0, true},
		{"1.20.4-2.3.1", "1.21.1-2.3.1", 0, true},
		{"0.15.2+1.21.1", "0.15.2+1.21.4", 0, true},
		{"1.0+abc", "abc+1.0", 0, true},
		{"2.1.0+build.5", "build.5+2.1.0", 0, true},
		{"5", "5", 0, true},
		{"v5", "5", 0, true},
		{"5", "6", 0, false},
		{"2024", "2024", 0, true},
		{"2024", "2025", 0, false},
		{"Build_5", "build-5", 0, false},
		{"build-5", "build-5", 0, true},
		{"release", "release", 0, true},
		{"release", "Release", 0, true},
		{"alpha", "beta", 0, false},
		{"", "1.0", 0, false},
		{"1.0", "", 0, false},
		{"", "", 0, true},
		{"2024.01.15", "2024.02.01", 1, true},
		{"20240115", "20240201", 0, false},
		{"1.02", "1.2", 0, true},
		{"7.3.6-b1234", "7.3.6-b1250", 1, true},
		{"EssentialsX-2.20.1", "EssentialsX-2.21.0", 1, true},
		{"LuckPerms-Bukkit-5.4.131", "LuckPerms-Bukkit-5.4.140", 1, true},
		{"1.0.0+20240101", "1.0.0+20240201", 0, true},
		{"1.0.0+abc", "1.0.0+def", 0, true},
		{"1.2.3.4", "1.2.3.5", 1, true},
		{"1.2.3.4", "1.2.3", -1, true},
		{" 1.2.3 ", "1.2.3", 0, true},
		{"1.0.0-ß", "1.0.0", 0, true},
		{"3.1.2-1.21", "3.1.3-1.21", 1, true},
		{"v2.0.0-beta.1", "v2.0.0-beta.2", 1, true},
		{"1.21.1", "1.21.4", 1, true},
		{"4.0.0", "3.9.9", -1, true},
		{"10.0", "9.9.9", -1, true},
		{"0.15.2 (MC 1.21.1)", "0.16.0 (MC 1.21.1)", 1, true},
		{"Sodium 0.5.8 for 1.21.1", "Sodium 0.6.0 for 1.21.1", 1, true},
		{"Sodium 0.5.8 for 1.21.1", "Sodium 0.5.8 for 1.21.1", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.current+" -> "+tt.latest, func(t *testing.T) {
			got, confident := compareLatestToCurrent(tt.current, tt.latest)
			if got != tt.want || confident != tt.confident {
				t.Fatalf("compareLatestToCurrent(%q, %q) = %d, %t; want %d, %t", tt.current, tt.latest, got, confident, tt.want, tt.confident)
			}
		})
	}
}
