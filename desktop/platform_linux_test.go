//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeSupply(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	for p, v := range files {
		full := filepath.Join(root, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(v+"\n"), 0o644)
	}
	return root
}

func TestBatteryLow(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"low on battery", map[string]string{"BAT0/type": "Battery", "BAT0/capacity": "7", "AC/type": "Mains", "AC/online": "0"}, true},
		{"low but charging", map[string]string{"BAT0/type": "Battery", "BAT0/capacity": "7", "AC/type": "Mains", "AC/online": "1"}, false},
		{"fine", map[string]string{"BAT0/type": "Battery", "BAT0/capacity": "55", "AC/type": "Mains", "AC/online": "0"}, false},
		{"desktop", map[string]string{"AC/type": "Mains", "AC/online": "1"}, false},
	}
	for _, c := range cases {
		if got := batteryLowIn(fakeSupply(t, c.files)); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}
