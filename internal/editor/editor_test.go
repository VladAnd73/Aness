package editor

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLocation(t *testing.T) {
	cases := []struct {
		arg  string
		want Location
	}{
		{"a.go", Location{Path: "/w/a.go"}},
		{"a.go:12", Location{Path: "/w/a.go", Line: 12}},
		{"a.go:12:3", Location{Path: "/w/a.go", Line: 12, Col: 3}},
		{"/abs/b.go:7", Location{Path: "/abs/b.go", Line: 7}},
		{"../up/c.go:1:1", Location{Path: "/up/c.go", Line: 1, Col: 1}},
		{"odd:name.go:4", Location{Path: "/w/odd:name.go", Line: 4}},
		{"x.go:1:2:3", Location{Path: "/w/x.go:1", Line: 2, Col: 3}},
		{"x.go:0", Location{Path: "/w/x.go:0"}},
	}
	for _, c := range cases {
		got, err := ParseLocation(c.arg, "/w")
		if err != nil {
			t.Fatalf("%q: %v", c.arg, err)
		}
		if got != c.want {
			t.Errorf("%q: got %+v want %+v", c.arg, got, c.want)
		}
	}
	for _, bad := range []string{"", ":12"} {
		if _, err := ParseLocation(bad, "/w"); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestLocationString(t *testing.T) {
	if s := (Location{Path: "/a", Line: 3, Col: 4}).String(); s != "/a:3:4" {
		t.Fatal(s)
	}
	if s := (Location{Path: "/a"}).String(); s != "/a" {
		t.Fatal(s)
	}
}

// shortDir keeps socket paths under the 108-byte sun_path limit;
// t.TempDir paths can exceed it.
func shortDir(t *testing.T) string {
	d, err := os.MkdirTemp("", "ed")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func listen(t *testing.T, path string) *net.UnixListener {
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l
}

func staleSocket(t *testing.T, path string) {
	l := listen(t, path)
	l.SetUnlinkOnClose(false)
	l.Close()
}

func TestLiveSocketsSkipsStale(t *testing.T) {
	d := shortDir(t)
	old := filepath.Join(d, "vscode-ipc-old.sock")
	stale := filepath.Join(d, "vscode-ipc-stale.sock")
	fresh := filepath.Join(d, "vscode-ipc-new.sock")
	l1 := listen(t, old)
	defer l1.Close()
	staleSocket(t, stale)
	l2 := listen(t, fresh)
	defer l2.Close()
	os.WriteFile(filepath.Join(d, "vscode-ipc-plain.sock"), nil, 0o600)
	now := time.Now()
	os.Chtimes(old, now.Add(-time.Hour), now.Add(-time.Hour))
	os.Chtimes(stale, now.Add(time.Hour), now.Add(time.Hour))

	if !Alive(fresh) || Alive(stale) {
		t.Fatalf("probe: fresh=%v stale=%v", Alive(fresh), Alive(stale))
	}
	got, err := LiveSockets(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != fresh || got[1] != old {
		t.Fatalf("got %v", got)
	}
}

func TestLiveSocketsNoneLive(t *testing.T) {
	d := shortDir(t)
	staleSocket(t, filepath.Join(d, "vscode-ipc-a.sock"))
	_, err := LiveSockets(d)
	if !errors.Is(err, ErrNoEditor) {
		t.Fatalf("want ErrNoEditor, got %v", err)
	}
}

func writeCLI(t *testing.T, home, hash, script string) string {
	dir := filepath.Join(home, ".vscode-server", "cli", "servers", "Stable-"+hash, "server", "bin", "remote-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "code")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindCLINewestWithCLI(t *testing.T) {
	home := t.TempDir()
	if _, err := FindCLI(home); !errors.Is(err, ErrNoEditor) {
		t.Fatalf("empty home: %v", err)
	}
	older := writeCLI(t, home, "old", "#!/bin/sh\n")
	newer := writeCLI(t, home, "new", "#!/bin/sh\n")
	partial := filepath.Join(home, ".vscode-server", "cli", "servers", "Stable-partial")
	os.MkdirAll(partial, 0o755)
	now := time.Now()
	os.Chtimes(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(older)))), now.Add(-time.Hour), now.Add(-time.Hour))
	os.Chtimes(partial, now.Add(time.Hour), now.Add(time.Hour))
	got, err := FindCLI(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Fatalf("got %s want %s", got, newer)
	}
}

func TestVSCodeOpenFallsThroughToNextSocket(t *testing.T) {
	home := t.TempDir()
	rt := shortDir(t)
	first := filepath.Join(rt, "vscode-ipc-1.sock")
	second := filepath.Join(rt, "vscode-ipc-2.sock")
	l1 := listen(t, first)
	defer l1.Close()
	l2 := listen(t, second)
	defer l2.Close()
	now := time.Now()
	os.Chtimes(second, now.Add(-time.Hour), now.Add(-time.Hour))

	log := filepath.Join(home, "calls")
	writeCLI(t, home, "h", "#!/bin/sh\necho \"$VSCODE_IPC_HOOK_CLI $*\" >> "+log+"\n"+
		"[ \"$VSCODE_IPC_HOOK_CLI\" = \""+first+"\" ] && { echo window gone >&2; exit 1; }\nexit 0\n")

	v := &VSCode{Home: home, RuntimeDir: rt, Timeout: 5 * time.Second}
	if err := v.Open(Location{Path: "/w/a.go", Line: 9, Col: 2}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	want := first + " --reuse-window --goto /w/a.go:9:2\n" + second + " --reuse-window --goto /w/a.go:9:2\n"
	if string(b) != want {
		t.Fatalf("calls:\n%s\nwant:\n%s", b, want)
	}
}

func TestVSCodeOpenAllRefuse(t *testing.T) {
	home := t.TempDir()
	rt := shortDir(t)
	l := listen(t, filepath.Join(rt, "vscode-ipc-1.sock"))
	defer l.Close()
	writeCLI(t, home, "h", "#!/bin/sh\necho nope >&2\nexit 1\n")
	err := (&VSCode{Home: home, RuntimeDir: rt}).Open(Location{Path: "/a"})
	if !errors.Is(err, ErrNoEditor) || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("got %v", err)
	}
}

func TestVSCodeOpenTimeoutKillsWrappedChild(t *testing.T) {
	home := t.TempDir()
	rt := shortDir(t)
	hung := filepath.Join(rt, "vscode-ipc-1.sock")
	ok := filepath.Join(rt, "vscode-ipc-2.sock")
	l1 := listen(t, hung)
	defer l1.Close()
	l2 := listen(t, ok)
	defer l2.Close()
	now := time.Now()
	os.Chtimes(ok, now.Add(-time.Hour), now.Add(-time.Hour))
	writeCLI(t, home, "h", "#!/bin/sh\n[ \"$VSCODE_IPC_HOOK_CLI\" = \""+hung+"\" ] && { sleep 30; true; }\nexit 0\n")

	v := &VSCode{Home: home, RuntimeDir: rt, Timeout: 300 * time.Millisecond}
	start := time.Now()
	if err := v.Open(Location{Path: "/a"}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Open took %v; timeout did not bound the wrapped child", d)
	}
}

func TestLiveSocketsStaleCountIgnoresPlainFiles(t *testing.T) {
	d := shortDir(t)
	staleSocket(t, filepath.Join(d, "vscode-ipc-a.sock"))
	os.WriteFile(filepath.Join(d, "vscode-ipc-plain.sock"), nil, 0o600)
	_, err := LiveSockets(d)
	if err == nil || !strings.Contains(err.Error(), "(1 stale)") {
		t.Fatalf("got %v", err)
	}
}
