package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSporeTOML(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadConfig(t *testing.T) {
	cases := []struct {
		name    string
		toml    string
		want    bool
		wantErr bool
	}{
		{"absent section", "[fleet]\naccount_tier = \"max\"\n", false, false},
		{"on", "[editor]\nauto_connect_ide = true\n", true, false},
		{"off", "[editor]\nauto_connect_ide = false\n", false, false},
		{"not a bool", "[editor]\nauto_connect_ide = 1\n", false, true},
		{"unknown key", "[editor]\nautoconnect = true\n", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig(writeSporeTOML(t, tc.toml))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if cfg.AutoConnectIDE != tc.want {
				t.Errorf("AutoConnectIDE = %v, want %v", cfg.AutoConnectIDE, tc.want)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil || cfg.AutoConnectIDE {
		t.Errorf("got %+v, %v; want zero, nil", cfg, err)
	}
}
