package fleet

import (
	"encoding/pem"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// serveBareOverHTTPS serves the bare repo through git http-backend on
// a TLS httptest server and returns its https URL. The app push only
// takes an https transport, so the fixtures reach a local repo over
// real https instead of an insteadOf to a filesystem path.
func serveBareOverHTTPS(t *testing.T, bare string) string {
	t.Helper()
	execPath, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Fatalf("git --exec-path: %v", err)
	}
	if out, err := exec.Command("git", "-C", bare, "config", "http.receivepack", "true").CombinedOutput(); err != nil {
		t.Fatalf("enable receive-pack: %v: %s", err, out)
	}
	srv := httptest.NewTLSServer(&cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + filepath.Dir(bare), "GIT_HTTP_EXPORT_ALL=1"},
	})
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSL_CAINFO", ca)
	return srv.URL + "/" + filepath.Base(bare)
}
