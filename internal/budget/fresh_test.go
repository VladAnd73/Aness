package budget

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRequireFreshRejectsAShortLivedTokenWithoutTouchingTheFile(t *testing.T) {
	creds := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, creds, "at-old", "rt-old", time.Now().Add(5*time.Minute).UnixMilli())
	before, err := os.ReadFile(creds)
	if err != nil {
		t.Fatal(err)
	}

	if err := RequireFresh(creds, 15*time.Minute); err == nil {
		t.Fatal("RequireFresh accepted a token with five minutes left")
	}
	after, err := os.ReadFile(creds)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("RequireFresh rewrote the host credentials file")
	}
}

func TestRequireFreshAcceptsALongLivedToken(t *testing.T) {
	creds := filepath.Join(t.TempDir(), ".credentials.json")
	writeCredentials(t, creds, "at-old", "rt-old", time.Now().Add(2*time.Hour).UnixMilli())

	if err := RequireFresh(creds, 15*time.Minute); err != nil {
		t.Fatalf("RequireFresh: %v", err)
	}
}
