package budget

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func tokenServer(t *testing.T, calls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		_, _ = fmt.Fprintln(w, `{"access_token":"at-new","refresh_token":"rt-new","expires_in":28800}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEnsureFreshRefreshesWhenLifeIsShort(t *testing.T) {
	creds := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, creds, "at-old", "rt-old", time.Now().Add(5*time.Minute).UnixMilli())
	var calls int32
	srv := tokenServer(t, &calls)
	swapURLs(t, srv.URL+"/usage-unused", srv.URL)

	if err := EnsureFresh(context.Background(), creds, 15*time.Minute); err != nil {
		t.Fatalf("EnsureFresh: %v", err)
	}
	cf, err := loadCredentials(creds)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || cf.oauth.AccessToken != "at-new" || cf.oauth.RefreshToken != "rt-new" {
		t.Errorf("calls=%d oauth=%+v; want one refresh written back", calls, cf.oauth)
	}
	if left := time.Until(time.UnixMilli(cf.oauth.ExpiresAt)); left < 7*time.Hour {
		t.Errorf("new expiry only %v away", left)
	}
	if _, ok := cf.raw["mcpOAuth"]; !ok {
		t.Error("refresh dropped the other top-level keys")
	}
}

func TestEnsureFreshLeavesALongLivedTokenAlone(t *testing.T) {
	creds := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, creds, "at-old", "rt-old", time.Now().Add(2*time.Hour).UnixMilli())
	var calls int32
	srv := tokenServer(t, &calls)
	swapURLs(t, srv.URL+"/usage-unused", srv.URL)

	if err := EnsureFresh(context.Background(), creds, 15*time.Minute); err != nil {
		t.Fatalf("EnsureFresh: %v", err)
	}
	if calls != 0 {
		t.Errorf("refreshed a token with two hours left (%d calls)", calls)
	}
}

func TestEnsureFreshFailsWhenRefreshFails(t *testing.T) {
	creds := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, creds, "at-old", "rt-old", time.Now().Add(time.Minute).UnixMilli())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
	}))
	defer srv.Close()
	swapURLs(t, srv.URL+"/usage-unused", srv.URL)

	if err := EnsureFresh(context.Background(), creds, 15*time.Minute); err == nil {
		t.Error("EnsureFresh hid a failed refresh")
	}
}
