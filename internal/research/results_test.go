package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendRowSanitizesDescription(t *testing.T) {
	p := filepath.Join(t.TempDir(), "results.tsv")
	at := time.Date(2026, 11, 10, 1, 2, 3, 0, time.UTC)
	if err := AppendRow(p, Row{Try: 1, At: at, Score: 40, Status: "reset", Bytes: 104, Commit: "c1", Description: "a\tb\nc"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendRow(p, Row{Try: 2, At: at, Status: "crash", Bytes: 0, Description: "x"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	want := "try\tat\tscore\tstatus\tbytes\tcommit\tdescription\n" +
		"1\t2026-11-10T01:02:03Z\t40.0\treset\t104\tc1\ta b c\n" +
		"2\t2026-11-10T01:02:03Z\t-\tcrash\t0\t\tx\n"
	if string(b) != want {
		t.Errorf("results.tsv =\n%q\nwant\n%q", b, want)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	rows, err := ReadRows(p)
	if err != nil || len(rows) != 2 || rows[0].Score != 40 || rows[0].Description != "a b c" || rows[1].Status != "crash" || !rows[1].At.Equal(at) {
		t.Errorf("ReadRows = %+v, %v", rows, err)
	}
	if strings.Count(string(b), "try\t") != 1 {
		t.Errorf("header written twice")
	}
}
