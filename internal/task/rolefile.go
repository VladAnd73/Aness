package task

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EngineerAuthor is the `author` value every engineer response must
// carry. It equals the SPORE_ROLE the engineer pane is spawned with.
const EngineerAuthor = "engineer"

// InvalidSuffix marks a role file the drive moved aside after it
// failed validation. listRounds ignores it (no `.json` suffix).
const InvalidSuffix = ".invalid"

// legacyMarker lists, one relative path per line, the role files that
// existed when a pre-validation tree was first driven by this binary.
// Those files may omit `author` and carry unknown keys: rejecting them
// would rewind an in-flight loop that already consumed them.
const legacyMarker = "legacy-files"

// ErrInvalidRoleFile is wrapped by every validation failure so callers
// can tell a bad file (skip it, bounce it) from an I/O error.
var ErrInvalidRoleFile = errors.New("invalid role file")

// InvalidRoleFileError carries the reason a role file failed
// validation.
type InvalidRoleFileError struct {
	Path   string
	Reason string
}

func (e *InvalidRoleFileError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Reason)
}

func (e *InvalidRoleFileError) Unwrap() error { return ErrInvalidRoleFile }

// RoleFile names one on-disk engineer response or review verdict.
// Instance is empty for engineer responses.
type RoleFile struct {
	Path     string
	Rel      string
	Instance ReviewerInstance
	Round    int
}

// Author returns the `author` value the file must carry.
func (f RoleFile) Author() string {
	if f.Instance == "" {
		return EngineerAuthor
	}
	return string(f.Instance)
}

// ListRoleFiles returns every engineer response and review file under
// the task tree, engineer first, then A, then B, each in round order.
func ListRoleFiles(projectRoot, slug string) ([]RoleFile, error) {
	root := RoleTaskDir(projectRoot, slug)
	var out []RoleFile
	eng, err := ListEngineerRounds(projectRoot, slug)
	if err != nil {
		return nil, err
	}
	for _, r := range eng {
		p := EngineerResponsePath(projectRoot, slug, r)
		out = append(out, RoleFile{Path: p, Rel: relTo(root, p), Round: r})
	}
	for _, inst := range []ReviewerInstance{ReviewerA, ReviewerB} {
		rounds, err := ListReviewRounds(projectRoot, slug, inst)
		if err != nil {
			return nil, err
		}
		for _, r := range rounds {
			p := ReviewPath(projectRoot, slug, inst, r)
			out = append(out, RoleFile{Path: p, Rel: relTo(root, p), Instance: inst, Round: r})
		}
	}
	return out, nil
}

// ValidateRoleFile reads f and checks it against its schema. Returns
// nil when valid, an error wrapping ErrInvalidRoleFile when the
// content is bad, or the underlying I/O error.
func ValidateRoleFile(projectRoot, slug string, f RoleFile) error {
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	legacy, err := isLegacyFile(projectRoot, slug, f.Rel)
	if err != nil {
		return err
	}
	if f.Instance == "" {
		_, err = ParseEngineerResponse(f.Path, b, legacy)
	} else {
		_, err = ParseReview(f.Path, b, f.Instance, legacy)
	}
	return err
}

// ParseReview decodes a reviewer verdict. Strict mode rejects unknown
// keys, trailing data, a missing summary, and an author other than
// the reviewer instance. Legacy mode (grandfathered files) tolerates
// unknown keys and a missing author. Both enforce the verdict enum.
func ParseReview(path string, b []byte, instance ReviewerInstance, legacy bool) (Review, error) {
	var r Review
	if err := decodeRoleJSON(b, &r, legacy); err != nil {
		return r, invalid(path, err.Error())
	}
	if r.Verdict != VerdictApprove && r.Verdict != VerdictRequestChanges {
		return r, invalid(path, fmt.Sprintf("verdict %q not one of %q, %q", r.Verdict, VerdictApprove, VerdictRequestChanges))
	}
	if err := checkAuthor(path, r.Author, string(instance), legacy); err != nil {
		return r, err
	}
	if !legacy && strings.TrimSpace(r.Summary) == "" {
		return r, invalid(path, `missing required field "summary"`)
	}
	return r, nil
}

// ParseEngineerResponse decodes an engineer response with the same
// strict / legacy split as ParseReview.
func ParseEngineerResponse(path string, b []byte, legacy bool) (EngineerResponse, error) {
	var r EngineerResponse
	if err := decodeRoleJSON(b, &r, legacy); err != nil {
		return r, invalid(path, err.Error())
	}
	if err := checkAuthor(path, r.Author, EngineerAuthor, legacy); err != nil {
		return r, err
	}
	return r, nil
}

func decodeRoleJSON(b []byte, v any, legacy bool) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if !legacy {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty file")
		}
		return fmt.Errorf("decode: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the JSON object")
	}
	return nil
}

func checkAuthor(path, got, want string, legacy bool) error {
	if got == "" {
		if legacy {
			return nil
		}
		return invalid(path, fmt.Sprintf(`missing required field "author" (want %q)`, want))
	}
	if got != want {
		return invalid(path, fmt.Sprintf("author %q does not match the file's role %q", got, want))
	}
	return nil
}

func invalid(path, reason string) error {
	return &InvalidRoleFileError{Path: path, Reason: reason}
}

// QuarantinePath returns the first free move-aside name for path:
// `<path>.invalid`, then `<path>.invalid.2`, and so on. The second
// return is the number of earlier quarantines of the same path.
func QuarantinePath(path string) (string, int) {
	for n := 1; ; n++ {
		dest := path + InvalidSuffix
		if n > 1 {
			dest = fmt.Sprintf("%s.%d", dest, n)
		}
		if _, err := os.Lstat(dest); errors.Is(err, fs.ErrNotExist) {
			return dest, n - 1
		}
	}
}

// EnsureLegacyMarker records the role files present on the first drive
// of a task tree by this binary. Idempotent: an existing marker is left
// alone. A fresh tree records an empty list, so every file is strict.
func EnsureLegacyMarker(projectRoot, slug string) error {
	stateDir := filepath.Join(RoleTaskDir(projectRoot, slug), "state")
	marker := filepath.Join(stateDir, legacyMarker)
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	files, err := ListRoleFiles(projectRoot, slug)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f.Rel)
		b.WriteString("\n")
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	fh, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	if _, err := fh.WriteString(b.String()); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}

// isLegacyFile reports whether rel is grandfathered. With no marker on
// disk (a pre-upgrade tree not yet driven), every file counts as
// legacy: that is what the first drive will record.
func isLegacyFile(projectRoot, slug, rel string) (bool, error) {
	f, err := os.Open(filepath.Join(RoleTaskDir(projectRoot, slug), "state", legacyMarker))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if sc.Text() == rel {
			return true, nil
		}
	}
	return false, sc.Err()
}

func relTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(rel)
}
