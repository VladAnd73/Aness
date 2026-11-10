package research

import (
	"os"
	"path/filepath"
)

func appendReport(s Store, name, section string) error {
	f, err := os.OpenFile(filepath.Join(s.Dir(name), "report.md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(section); err != nil {
		return err
	}
	return f.Close()
}
