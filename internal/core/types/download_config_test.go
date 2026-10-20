package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadConfigLoadAndSave(t *testing.T) {
	for _, tc := range []struct {
		name, input   string
		hours, height int
	}{
		{"legacy", "[download]\nuse_aria2 = false\n", 24, 0},
		{"cap_only", "[download]\nmax_height = 1080\n", 24, 1080},
		{"custom", "[download]\noverall_timeout_hours = 48\nmax_height = 720\n", 48, 720},
		{"invalid", "[download]\noverall_timeout_hours = -1\nmax_height = -1\n", 24, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if c.Download.OverallTimeoutHours != tc.hours || c.Download.MaxHeight != tc.height {
				t.Fatalf("unexpected settings: %+v", c.Download)
			}
			if err := SaveConfig(c); err != nil {
				t.Fatal(err)
			}
			again, err := LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if again.Download != c.Download {
				t.Fatalf("download config changed after save: %+v", again.Download)
			}
		})
	}
}
