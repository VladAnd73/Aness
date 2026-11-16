package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAccountTier(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want string
	}{
		{"missing file", "", ""},
		{"no fleet section", "[coordinator]\nbrief = \"x.md\"\n", ""},
		{"no key", "[fleet]\nmax_workers = 3\n", ""},
		{"quoted", "[fleet]\naccount_tier = \"max\"\n", "max"},
		{"bare with comment", "[fleet]\naccount_tier = max # dogfood\n", "max"},
		{"other section after", "[fleet]\naccount_tier = \"sub\"\n[align]\nnotes = 10\n", "sub"},
		{"key outside fleet ignored", "[coordinator]\naccount_tier = \"max\"\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.toml != "" {
				if err := os.WriteFile(filepath.Join(root, "spore.toml"), []byte(tc.toml), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := LoadAccountTier(root); got != tc.want {
				t.Errorf("LoadAccountTier = %q, want %q", got, tc.want)
			}
		})
	}
}
