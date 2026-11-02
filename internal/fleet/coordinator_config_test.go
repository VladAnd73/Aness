package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCoordinatorTOML(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    CoordinatorConfig
		wantErr bool
	}{
		{
			name: "all keys",
			input: `
[coordinator]
driver = "claude"
model  = "opus"
brief  = "docs/helm.md"
`,
			want: CoordinatorConfig{Driver: "claude", Model: "opus", Brief: "docs/helm.md"},
		},
		{
			name:  "single quotes",
			input: "[coordinator]\ndriver = 'codex'\n",
			want:  CoordinatorConfig{Driver: "codex"},
		},
		{
			name: "comments and blanks",
			input: `
# leading comment
[coordinator]
# inline driver below
driver = "claude" # trailing comment
`,
			want: CoordinatorConfig{Driver: "claude"},
		},
		{
			name: "other section ignored",
			input: `
[fleet]
max_workers = 2
[coordinator]
driver = "claude"
`,
			want: CoordinatorConfig{Driver: "claude"},
		},
		{
			name:  "external_session_pattern",
			input: "[coordinator]\nexternal_session_pattern = \"^helm-.*\"\n",
			want:  CoordinatorConfig{ExternalSessionPattern: "^helm-.*"},
		},
		{
			name:  "unknown key is ignored, not fatal",
			input: "[coordinator]\ndriver = \"claude\"\nbogus = 1\n",
			want:  CoordinatorConfig{Driver: "claude"},
		},
		{
			name:    "malformed line errors",
			input:   "[coordinator]\nnoequals\n",
			wantErr: true,
		},
		{
			name:  "no section returns zero",
			input: "[fleet]\nmax_workers = 2\n",
			want:  CoordinatorConfig{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := parseCoordinatorTOML(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; result=%+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCoordinatorTOML: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// Finding 3C. The parser rejected any key it did not recognise, and
// every caller swallowed that error into a zero config. A host running
// a mix of spore versions therefore lost driver, model AND brief the
// moment a newer binary's key appeared in a shared spore.toml. A key
// from the future is not a reason to discard the present.
func TestParseCoordinatorTOML_AKeyFromTheFutureDoesNotDiscardTheSection(t *testing.T) {
	input := `
[coordinator]
driver = "claude"
model  = "opus"
brief  = "bootstrap/coordinator/role.md"
some_key_a_later_release_adds = 9
`
	got, warnings, err := parseCoordinatorTOML(input)
	if err != nil {
		t.Fatalf("parseCoordinatorTOML: %v", err)
	}
	want := CoordinatorConfig{
		Driver: "claude",
		Model:  "opus",
		Brief:  "bootstrap/coordinator/role.md",
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "some_key_a_later_release_adds") {
		t.Errorf("warnings = %v, want one naming the unknown key", warnings)
	}
}

// Same rule for a value this binary cannot use: the knob falls back to
// its default and the rest of the section survives.
func TestParseCoordinatorTOML_AnUnusableValueDoesNotDiscardTheSection(t *testing.T) {
	input := "[coordinator]\ndriver = \"claude\"\ninbox_watchdog_escalate_after = \"soon\"\n"
	got, warnings, err := parseCoordinatorTOML(input)
	if err != nil {
		t.Fatalf("parseCoordinatorTOML: %v", err)
	}
	if got.Driver != "claude" {
		t.Errorf("driver = %q, want %q", got.Driver, "claude")
	}
	if got.InboxWatchdogEscalateAfter != 0 {
		t.Errorf("escalate_after = %d, want 0 so the default applies", got.InboxWatchdogEscalateAfter)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "inbox_watchdog_escalate_after") {
		t.Errorf("warnings = %v, want one naming the key", warnings)
	}
}

func TestLoadCoordinatorConfigMissingFile(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadCoordinatorConfig(dir)
	if err != nil {
		t.Fatalf("LoadCoordinatorConfig on empty dir: %v", err)
	}
	if (got != CoordinatorConfig{}) {
		t.Errorf("expected zero CoordinatorConfig for missing spore.toml, got %+v", got)
	}
}

func TestLoadCoordinatorConfigReadsFile(t *testing.T) {
	dir := t.TempDir()
	body := []byte("[coordinator]\ndriver = \"codex\"\nmodel = \"gpt-5.5\"\nbrief = \"docs/helm.md\"\n")
	if err := os.WriteFile(filepath.Join(dir, "spore.toml"), body, 0o600); err != nil {
		t.Fatalf("write spore.toml: %v", err)
	}
	got, err := LoadCoordinatorConfig(dir)
	if err != nil {
		t.Fatalf("LoadCoordinatorConfig: %v", err)
	}
	want := CoordinatorConfig{Driver: "codex", Model: "gpt-5.5", Brief: "docs/helm.md"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestCoordinatorRolePathPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spore.toml"),
		[]byte("[coordinator]\nbrief = \"docs/helm.md\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("env wins", func(t *testing.T) {
		t.Setenv(CoordinatorRoleEnv, "/etc/role.md")
		if got := CoordinatorRolePath(dir); got != "/etc/role.md" {
			t.Errorf("env override ignored: got %q", got)
		}
	})

	t.Run("toml relative resolves against root", func(t *testing.T) {
		t.Setenv(CoordinatorRoleEnv, "")
		want := filepath.Join(dir, "docs/helm.md")
		if got := CoordinatorRolePath(dir); got != want {
			t.Errorf("toml relative: got %q, want %q", got, want)
		}
	})

	t.Run("toml absolute passes through", func(t *testing.T) {
		t.Setenv(CoordinatorRoleEnv, "")
		abs := filepath.Join(dir, "abs-helm.md")
		if err := os.WriteFile(filepath.Join(dir, "spore.toml"),
			[]byte("[coordinator]\nbrief = \""+abs+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := CoordinatorRolePath(dir); got != abs {
			t.Errorf("toml absolute: got %q, want %q", got, abs)
		}
	})

	t.Run("default fallback", func(t *testing.T) {
		empty := t.TempDir()
		t.Setenv(CoordinatorRoleEnv, "")
		want := filepath.Join(empty, "bootstrap", "coordinator", "role.md")
		if got := CoordinatorRolePath(empty); got != want {
			t.Errorf("default fallback: got %q, want %q", got, want)
		}
	})
}

func TestCoordinatorProviderModel(t *testing.T) {
	t.Run("env wins over toml", func(t *testing.T) {
		t.Setenv("SPORE_COORDINATOR_PROVIDER", "envprov")
		t.Setenv("SPORE_COORDINATOR_MODEL", "envmodel")
		cfg := CoordinatorConfig{Driver: "claude", Model: "opus"}
		if got := coordinatorProvider(cfg); got != "envprov" {
			t.Errorf("provider = %q, want envprov", got)
		}
		if got := coordinatorModel(cfg); got != "envmodel" {
			t.Errorf("model = %q, want envmodel", got)
		}
	})
	t.Run("toml fallback", func(t *testing.T) {
		t.Setenv("SPORE_COORDINATOR_PROVIDER", "")
		t.Setenv("SPORE_COORDINATOR_MODEL", "")
		cfg := CoordinatorConfig{Driver: "codex", Model: "gpt-5.5"}
		if got := coordinatorProvider(cfg); got != "codex" {
			t.Errorf("provider = %q, want codex", got)
		}
		if got := coordinatorModel(cfg); got != "gpt-5.5" {
			t.Errorf("model = %q, want gpt-5.5", got)
		}
	})
	t.Run("nothing set", func(t *testing.T) {
		t.Setenv("SPORE_COORDINATOR_PROVIDER", "")
		t.Setenv("SPORE_COORDINATOR_MODEL", "")
		if got := coordinatorProvider(CoordinatorConfig{}); got != "" {
			t.Errorf("provider = %q, want empty", got)
		}
		if got := coordinatorModel(CoordinatorConfig{}); got != "" {
			t.Errorf("model = %q, want empty", got)
		}
	})
}

func TestParseCoordinatorTOML_Caps(t *testing.T) {
	cfg, warnings, err := parseCoordinatorTOML("[coordinator]\nsoft_cap = 350000\nhard_cap = 400000\n")
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, warnings)
	}
	if cfg.SoftCap != 350000 || cfg.HardCap != 400000 {
		t.Errorf("caps = %d/%d, want 350000/400000", cfg.SoftCap, cfg.HardCap)
	}
	_, warnings, _ = parseCoordinatorTOML("[coordinator]\nsoft_cap = lots\n")
	if len(warnings) != 1 {
		t.Errorf("unusable soft_cap: warnings = %v, want one", warnings)
	}
}
