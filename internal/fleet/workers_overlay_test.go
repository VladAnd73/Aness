package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkersOverlay(t *testing.T, root, body string) string {
	t.Helper()
	path, err := WorkersOverlayPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(path, body); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadWorkersConfigOverlay(t *testing.T) {
	base := "[fleet.workers]\ndefault = \"claude\"\n[fleet.workers.ratio]\nclaude = 60\ncodex = 40\n[fleet.workers.rules]\nheavy = \"claude\"\n"
	cases := []struct {
		name    string
		overlay string
		want    WorkersConfig
	}{
		{
			name: "none",
			want: WorkersConfig{Default: "claude", Ratio: map[string]int{"claude": 60, "codex": 40}, Rules: map[string]string{"heavy": "claude"}},
		},
		{
			name:    "default only",
			overlay: "[fleet.workers]\ndefault = \"codex\"\n",
			want:    WorkersConfig{Default: "codex", Ratio: map[string]int{"claude": 60, "codex": 40}, Rules: map[string]string{"heavy": "claude"}},
		},
		{
			name:    "ratio replaces whole map",
			overlay: "[fleet.workers.ratio]\ncodex = 100\n",
			want:    WorkersConfig{Default: "claude", Ratio: map[string]int{"codex": 100}, Rules: map[string]string{"heavy": "claude"}},
		},
		{
			name:    "empty rules table clears rules",
			overlay: "[fleet.workers.rules]\n",
			want:    WorkersConfig{Default: "claude", Ratio: map[string]int{"claude": 60, "codex": 40}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte(base), 0o644); err != nil {
				t.Fatal(err)
			}
			if c.overlay != "" {
				writeWorkersOverlay(t, root, c.overlay)
			}
			got, err := LoadWorkersConfig(root)
			if err != nil {
				t.Fatalf("LoadWorkersConfig: %v", err)
			}
			if !workersEqual(got, c.want) {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestLoadWorkersConfigOverlayBadKey(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	path := writeWorkersOverlay(t, root, "[fleet.workers]\nbogus = \"codex\"\n")
	_, err := LoadWorkersConfig(root)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("err = %v, want unknown key error naming %s", err, path)
	}
}

func TestSetWorkerDefaultHostSideOnly(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()

	if a, src, err := EffectiveWorkerDefault(root); err != nil || a != "claude" || src != WorkerDefaultFromKernel {
		t.Fatalf("initial = %q %q %v, want claude kernel default", a, src, err)
	}
	path, err := SetWorkerDefault(root, "codex")
	if err != nil {
		t.Fatalf("SetWorkerDefault: %v", err)
	}
	if a, src, err := EffectiveWorkerDefault(root); err != nil || a != "codex" || src != WorkerDefaultFromOverlay {
		t.Fatalf("after set = %q %q %v, want codex overlay", a, src, err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("overlay mode = %v, want 0600", st.Mode().Perm())
	}
	if dst, _ := os.Stat(filepath.Dir(path)); dst.Mode().Perm() != 0o700 {
		t.Errorf("overlay dir mode = %v, want 0700", dst.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "spore.toml")); !os.IsNotExist(err) {
		t.Errorf("spore.toml exists after set (err %v); must never be written", err)
	}

	if _, err := SetWorkerDefault(root, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("empty overlay not removed (err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spore.toml")); !os.IsNotExist(err) {
		t.Errorf("spore.toml exists after clear (err %v)", err)
	}
}

func TestSetWorkerDefaultClearKeepsOtherTables(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte("[fleet.workers]\ndefault = \"codex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeWorkersOverlay(t, root, "[fleet.workers]\ndefault = \"claude\"\n\n[fleet.workers.ratio]\ncodex = 1\n")
	if _, err := SetWorkerDefault(root, ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("overlay removed though ratio remains: %v", err)
	}
	if strings.Contains(string(b), "default") || !strings.Contains(string(b), "codex = 1") {
		t.Errorf("overlay = %q, want ratio only", b)
	}
	if a, src, _ := EffectiveWorkerDefault(root); a != "codex" || src != WorkerDefaultFromSporeTOML {
		t.Errorf("effective = %q %q, want codex from spore.toml", a, src)
	}
}
