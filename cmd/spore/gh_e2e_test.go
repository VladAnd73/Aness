package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGitHub accepts only an RS256 app JWT signed by pub with iss=appID,
// posted to the installation's access_tokens endpoint, the way GitHub does.
func fakeGitHub(t *testing.T, pub *rsa.PublicKey, appID, installPath, token string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != installPath {
			http.Error(w, "not found: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		jwt, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		parts := strings.Split(jwt, ".")
		if !ok || len(parts) != 3 {
			http.Error(w, "no bearer jwt", http.StatusUnauthorized)
			return
		}
		enc := base64.RawURLEncoding
		sig, err := enc.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err != nil || rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig) != nil {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		raw, _ := enc.DecodeString(parts[1])
		var claims struct {
			Iss string `json:"iss"`
			Exp int64  `json:"exp"`
		}
		if json.Unmarshal(raw, &claims) != nil || claims.Iss != appID || claims.Exp <= time.Now().Unix() {
			http.Error(w, "bad claims: "+string(raw), http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      token,
			"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Given a throwaway app key and a github.toml written by `spore gh config
// --set`, the built binary mints a token from a GitHub that verifies the
// JWT signature, caches it, and `spore gh exec` hands it to git and gh.
func TestGHBinaryMintsTokenEndToEnd(t *testing.T) {
	bin := buildSporeBin(t)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemPath := filepath.Join(t.TempDir(), "app.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(pemPath, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	const minted = "ghs_e2e_minted"
	var hits atomic.Int32
	srv := fakeGitHub(t, &key.PublicKey, "41", "/app/installations/42/access_tokens", minted, &hits)

	root := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/o/r.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	state := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "SPORE_") || strings.HasPrefix(kv, "GH_") ||
			strings.HasPrefix(kv, "GIT_CONFIG_") || strings.HasPrefix(kv, "XDG_STATE_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "XDG_STATE_HOME="+state)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, append([]string{"gh"}, args...)...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			var stderr []byte
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = ee.Stderr
			}
			t.Fatalf("spore gh %v: %v\nstdout: %s\nstderr: %s", args, err, out, stderr)
		}
		return string(out)
	}

	run("config", "--set", "app_id=41", "--set", "installation_id=42",
		"--set", "private_key="+pemPath, "--set", "api_url="+srv.URL)
	cfg := filepath.Join(state, "spore", "proj", "github.toml")
	info, err := os.Stat(cfg)
	if err != nil {
		t.Fatalf("github.toml not at %s: %v", cfg, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("github.toml mode %o, want 600", perm)
	}

	if got := strings.TrimSpace(run("token")); got != minted {
		t.Fatalf("token = %q, want %q", got, minted)
	}
	if got := strings.TrimSpace(run("token")); got != minted {
		t.Fatalf("cached token = %q, want %q", got, minted)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("fake GitHub hit %d times, want 1 (second token from cache)", n)
	}

	out := run("exec", "--", "sh", "-c", `echo "$GH_TOKEN"; git config --get-all credential.helper`)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 || lines[0] != minted {
		t.Fatalf("exec output = %q, want token then credential helpers", out)
	}
	helpers := lines[len(lines)-2:]
	if helpers[0] != "" || !strings.Contains(helpers[1], "x-access-token") {
		t.Fatalf("credential helpers = %q, want reset then x-access-token helper", helpers)
	}
}
