package research

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

// Row is one results.tsv line. Status is baseline, kept, reset,
// rejected-length or crash.
type Row struct {
	Try                 int
	At                  time.Time
	Score               float64
	Status              string
	Bytes               int
	Commit, Description string
}

var resultsColumns = []string{"try", "at", "score", "status", "bytes", "commit", "description"}

// editorColumns is all the editor may learn about earlier tries.
var editorColumns = []string{"try", "score", "status", "bytes", "description"}

func scored(status string) bool { return status != "crash" && status != "rejected-length" }

func (r Row) cell(col string) string {
	switch col {
	case "try":
		return strconv.Itoa(r.Try)
	case "at":
		return r.At.UTC().Format(time.RFC3339)
	case "score":
		if !scored(r.Status) {
			return "-"
		}
		return fmt.Sprintf("%.1f", r.Score)
	case "status":
		return r.Status
	case "bytes":
		return strconv.Itoa(r.Bytes)
	case "commit":
		return r.Commit
	case "description":
		return strings.Map(func(c rune) rune {
			if c == '\t' || c == '\n' || c == '\r' {
				return ' '
			}
			return c
		}, r.Description)
	}
	return ""
}

func formatRow(r Row, cols []string) string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		cells[i] = r.cell(c)
	}
	return strings.Join(cells, "\t") + "\n"
}

// AppendRow writes the header with the first row, and each row with a
// single write, so an interrupted night never leaves half a line.
func AppendRow(path string, r Row) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	line := formatRow(r, resultsColumns)
	if fi.Size() == 0 {
		line = strings.Join(resultsColumns, "\t") + "\n" + line
	}
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return f.Close()
}

// ReadRows reads results.tsv by its header names. A missing file is no
// rows.
func ReadRows(path string) ([]Row, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	col := map[string]int{}
	for i, h := range strings.Split(lines[0], "\t") {
		col[h] = i
	}
	var rows []Row
	for n, line := range lines[1:] {
		cells := strings.Split(line, "\t")
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(cells) {
				return cells[i]
			}
			return ""
		}
		r := Row{Status: get("status"), Commit: get("commit"), Description: get("description")}
		if r.Try, err = strconv.Atoi(get("try")); err != nil {
			return nil, fmt.Errorf("research: %s line %d: try: %v", path, n+2, err)
		}
		r.Bytes, _ = strconv.Atoi(get("bytes"))
		r.At, _ = time.Parse(time.RFC3339, get("at"))
		if s := get("score"); s != "-" {
			r.Score, _ = strconv.ParseFloat(s, 64)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// writeEditorResults writes the score log the editor sees: the
// editorColumns of every row and nothing the judge said.
func writeEditorResults(path string, rows []Row) error {
	var b strings.Builder
	b.WriteString(strings.Join(editorColumns, "\t") + "\n")
	for _, r := range rows {
		b.WriteString(formatRow(r, editorColumns))
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
