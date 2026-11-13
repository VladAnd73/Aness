package main

import (
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

const hookTestCredentials = `{
  "claudeAiOauth": {
    "accessToken": "OLD-AT",
    "refreshToken": "OLD-RT",
    "expiresAt": 1,
    "refreshTokenExpiresAt": 4102444800000,
    "scopes": ["user:inference"],
    "subscriptionType": "max",
    "rateLimitTier": "default_claude_max_20x"
  }
}
`

// Bug 3, end to end: six coordinator Stop hooks against a fake Anthropic
// API whose /usage answers 401 once, then 429. The hook used to rotate
// the login on the 401, rewrite the credentials file without
// refreshTokenExpiresAt, then hit /usage on every Stop.
func TestReplenishHookLeavesTheLoginAloneAndBacksOff(t *testing.T) {
	bin := sharedSporeBin(t)
	var usage, token atomic.Int32
	proxyEnv := fakeAnthropic(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/oauth/token" {
			token.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"NEW-AT","refresh_token":"NEW-RT","expires_in":3600}`))
			return
		}
		if usage.Add(1) == 1 {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))

	env := e2eEnv(t)
	home := envValue(env, "HOME")
	creds := filepath.Join(home, ".claude", ".credentials.json")
	writeFile(t, creds, hookTestCredentials, 0o600)
	root := gitInitProject(t)
	coordState := filepath.Join(t.TempDir(), "coordinator")
	env = append(env, proxyEnv...)
	env = append(env,
		"AGENT_BUDGET_MODE=subscription",
		"SPORE_TASK_SLUG=coordinator",
		"SPORE_COORDINATOR_STATE_DIR="+coordState,
		"SPORE_TASK_INBOX="+filepath.Join(coordState, filepath.Base(root), "inbox"),
		"SPORE_PROJECT_ROOT="+root,
	)

	for i := 0; i < 6; i++ {
		if res := runCLI(t, bin, root, env, "{}", "fleet", "replenish-hook"); res.code != 0 {
			t.Fatalf("hook %d exit %d: %s", i, res.code, res.stderr)
		}
	}
	if usage.Load() == 0 {
		t.Fatal("the fake API saw no /usage call; the harness is not wired")
	}
	if n := token.Load(); n != 0 {
		t.Errorf("the Stop hook called the OAuth token endpoint %d time(s)", n)
	}
	if got, _ := os.ReadFile(creds); string(got) != hookTestCredentials {
		t.Errorf("credentials file rewritten by a Stop hook:\n%s", got)
	}
	if n := usage.Load(); n != 1 {
		t.Errorf("6 Stop hooks hit /usage %d times after a failure; want 1", n)
	}
}
