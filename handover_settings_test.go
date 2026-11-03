package spore

import (
	"encoding/json"
	"os"
	"testing"
)

func TestHandoverSettingsWireCommunicationHooks(t *testing.T) {
	raw, err := os.ReadFile("bootstrap/handover/settings.json")
	if err != nil {
		t.Fatalf("read handover settings: %v", err)
	}
	var settings struct {
		Hooks map[string][]handoverHookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("parse handover settings: %v", err)
	}

	if !hasCommand(settings.Hooks["PreToolUse"], "/home/spore/.claude/hooks/block-bg-bash.pl") {
		t.Fatal("handover settings lost block-bg-bash PreToolUse hook")
	}
	if !hasCommand(settings.Hooks["SessionStart"], "/home/spore/.claude/hooks/load-state-md.pl") {
		t.Fatal("handover settings lost load-state-md SessionStart hook")
	}
	if !hasAsync(settings.Hooks["Notification"], "/run/current-system/sw/bin/spore hooks notify-coordinator") {
		t.Fatal("handover settings missing async notify-coordinator Notification hook")
	}
	if !hasAsyncRewake(settings.Hooks["Stop"], "/run/current-system/sw/bin/spore hooks watch-inbox") {
		t.Fatal("handover settings missing asyncRewake watch-inbox Stop hook")
	}
	if !hasCommand(settings.Hooks["Stop"], "/run/current-system/sw/bin/spore coordinator token-monitor") {
		t.Fatal("handover settings missing coordinator token-monitor Stop hook")
	}
	if !hasCommand(settings.Hooks["Stop"], "/run/current-system/sw/bin/spore worker token-monitor") {
		t.Fatal("handover settings missing worker token-monitor Stop hook")
	}
	if !hasCommand(settings.Hooks["Stop"], "/run/current-system/sw/bin/spore fleet replenish-hook") {
		t.Fatal("handover settings missing fleet replenish-hook Stop hook")
	}
}

// A pending AskUserQuestion reaches the transcript JSONL only once a
// PreToolUse hook matches it; the hinterf console reads the question
// from there while it waits for an answer.
func TestHandoverSettingsFlushPendingAskUserQuestion(t *testing.T) {
	raw, err := os.ReadFile("bootstrap/handover/settings.json")
	if err != nil {
		t.Fatalf("read handover settings: %v", err)
	}
	var settings struct {
		Hooks map[string][]handoverHookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("parse handover settings: %v", err)
	}
	for _, group := range settings.Hooks["PreToolUse"] {
		if group.Matcher == "AskUserQuestion" && len(group.Hooks) > 0 {
			return
		}
	}
	t.Fatal("handover settings missing a PreToolUse hook matching AskUserQuestion")
}

func TestHandoverSettingsDeniesCrossSessionMessaging(t *testing.T) {
	raw, err := os.ReadFile("bootstrap/handover/settings.json")
	if err != nil {
		t.Fatalf("read handover settings: %v", err)
	}
	var settings struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
		Hooks map[string][]handoverHookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("parse handover settings: %v", err)
	}

	for _, want := range []string{"SendMessage", "ListAgents"} {
		if !contains(settings.Permissions.Deny, want) {
			t.Fatalf("handover settings permissions.deny missing %q; got %v", want, settings.Permissions.Deny)
		}
	}

	// Adding permissions must not clobber the wired hooks (scenario 2:
	// no-clobber). The dedicated hook test covers each entry; here we
	// just confirm the block survived alongside the new permissions.
	if len(settings.Hooks) == 0 {
		t.Fatal("handover settings lost its hooks block when permissions were added")
	}

	// crossSessionInbound is NOT in the pre-2.1.224 settings schema, so a
	// consumer on older claude-code would fail validation. It must stay
	// absent from this single static file shipped to every consumer.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("parse handover settings top-level: %v", err)
	}
	if _, ok := top["crossSessionInbound"]; ok {
		t.Fatal("handover settings ships crossSessionInbound unconditionally; breaks consumers on claude-code < 2.1.224")
	}
}

// TestBundledHandoverShipsCrossSessionDeny reads the settings through the
// embedded filesystem that ships inside the spore binary (the exact bytes
// `spore infect` installs onto a consumer), not the loose source file, so
// it proves the shipped artifact carries the deny list and parses cleanly.
func TestBundledHandoverShipsCrossSessionDeny(t *testing.T) {
	raw, err := BundledHandover.ReadFile("bootstrap/handover/settings.json")
	if err != nil {
		t.Fatalf("read embedded handover settings: %v", err)
	}
	var settings struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("embedded settings.json is not valid JSON: %v", err)
	}
	for _, want := range []string{"SendMessage", "ListAgents"} {
		if !contains(settings.Permissions.Deny, want) {
			t.Fatalf("embedded handover settings permissions.deny missing %q; got %v", want, settings.Permissions.Deny)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

type handoverHookGroup struct {
	Matcher string         `json:"matcher"`
	Hooks   []handoverHook `json:"hooks"`
}

type handoverHook struct {
	Command     string `json:"command"`
	Async       bool   `json:"async,omitempty"`
	AsyncRewake bool   `json:"asyncRewake,omitempty"`
}

func hasCommand(groups []handoverHookGroup, command string) bool {
	for _, group := range groups {
		for _, hook := range group.Hooks {
			if hook.Command == command {
				return true
			}
		}
	}
	return false
}

func hasAsync(groups []handoverHookGroup, command string) bool {
	for _, group := range groups {
		for _, hook := range group.Hooks {
			if hook.Command == command && hook.Async {
				return true
			}
		}
	}
	return false
}

func hasAsyncRewake(groups []handoverHookGroup, command string) bool {
	for _, group := range groups {
		for _, hook := range group.Hooks {
			if hook.Command == command && hook.AsyncRewake {
				return true
			}
		}
	}
	return false
}
