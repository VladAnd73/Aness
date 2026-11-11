package budget

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// The host's real file carries refreshTokenExpiresAt, which oauthBlock
// never modelled.
const hostShapedCredentials = `{
  "claudeAiOauth": {
    "accessToken": "OLD-AT",
    "refreshToken": "OLD-RT",
    "expiresAt": 1,
    "refreshTokenExpiresAt": 4102444800000,
    "scopes": ["user:inference"],
    "subscriptionType": "max",
    "rateLimitTier": "default_claude_max_20x"
  },
  "mcpOAuth": {"kept": true}
}
`

type fakeUsageAPI struct {
	usage, token atomic.Int32
}

// adviceEnv points Advice at a private state dir, fake credentials and a
// fake API whose /usage answers 401 once and 429 after that.
func adviceEnv(t *testing.T) (*fakeUsageAPI, string) {
	t.Helper()
	dir := t.TempDir()
	creds := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(creds, []byte(hostShapedCredentials), 0o600); err != nil {
		t.Fatal(err)
	}
	api := &fakeUsageAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			api.token.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"NEW-AT","refresh_token":"NEW-RT","expires_in":3600}`))
			return
		}
		if api.usage.Add(1) == 1 {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Retry-After", "280")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	swapURLs(t, srv.URL+"/usage", srv.URL+"/token")
	t.Setenv("AGENT_BUDGET_CREDS", creds)
	t.Setenv("AGENT_BUDGET_MODE", "subscription")
	t.Setenv("AGENT_BUDGET_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("AGENT_BUDGET_PROJECTS", filepath.Join(dir, "projects"))
	t.Setenv("AGENT_BUDGET_ACCOUNTS_DIR", filepath.Join(dir, "accounts"))
	t.Setenv("AGENT_BUDGET_USAGE_MIN_INTERVAL_SEC", "")
	return api, creds
}

// Bug 3: the coordinator's Stop hook asks for advice on every turn. A 401
// made it rotate the shared login and rewrite the credentials file
// without refreshTokenExpiresAt.
func TestAdviceNeverRotatesTheLoginOrRewritesCredentials(t *testing.T) {
	api, creds := adviceEnv(t)
	_ = Advice()
	if n := api.token.Load(); n != 0 {
		t.Fatalf("Advice called the OAuth token endpoint %d time(s); Claude Code owns the login", n)
	}
	got, err := os.ReadFile(creds)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(hostShapedCredentials)) {
		t.Fatalf("credentials file rewritten:\n%s", got)
	}
}

func TestAdviceThrottlesAFailedUsageRead(t *testing.T) {
	api, _ := adviceEnv(t)
	for i := 0; i < 6; i++ {
		_ = Advice()
	}
	if n := api.usage.Load(); n != 1 {
		t.Fatalf("6 Stop hooks in a row hit /usage %d times after a failure; want 1", n)
	}
}
