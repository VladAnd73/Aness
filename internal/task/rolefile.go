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
	"strconv"
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

// StepFiles describes the outputs of one loop step: Output is a path
// under the task tree with "{round}" standing for the round number.
// Author is the `author` value the shapes "verdict" and
// "engineer-response" check; shape "json" carries no author.
type StepFiles struct {
	Step, Output, Shape, Author string
	Role, Session               string // the pane a bounce wakes
	Required                    []string
}

// RoleFile names one on-disk engineer response or review verdict.
// Instance is empty for engineer responses. Step is set for a loop
// step's output that is not one of the built-in role files.
type RoleFile struct {
	Path     string
	Rel      string
	Instance ReviewerInstance
	Round    int
	Step     *StepFiles
}

// Author returns the `author` value the file must carry.
func (f RoleFile) Author() string {
	if f.Step != nil {
		return f.Step.Author
	}
	if f.Instance == "" {
		return EngineerAuthor
	}
	return string(f.Instance)
}

// ListRoleFiles returns the files a loop owns. With no steps that is
// every engineer response and review file under the task tree,
// engineer first, then A, then B, each in round order. With steps it
// is only their outputs, in step order, each owned by its step: a loop
// declares every file it reads, and a step may reuse a built-in path
// with its own shape.
func ListRoleFiles(projectRoot, slug string, steps ...StepFiles) ([]RoleFile, error) {
	root := RoleTaskDir(projectRoot, slug)
	if len(steps) > 0 {
		return listStepFiles(projectRoot, slug, root, steps)
	}
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

func listStepFiles(projectRoot, slug, root string, steps []StepFiles) ([]RoleFile, error) {
	var out []RoleFile
	seen := map[string]bool{}
	for i := range steps {
		sf := &steps[i]
		rounds, err := StepOutputRounds(projectRoot, slug, sf.Output)
		if err != nil {
			return nil, err
		}
		for _, r := range rounds {
			p := StepOutputPath(projectRoot, slug, sf.Output, r)
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, RoleFile{Path: p, Rel: relTo(root, p), Round: r, Step: sf})
		}
	}
	return out, nil
}

// StepOutputPath expands a step's output pattern for round.
func StepOutputPath(projectRoot, slug, output string, round int) string {
	rel := strings.ReplaceAll(output, "{round}", strconv.Itoa(round))
	return filepath.Join(RoleTaskDir(projectRoot, slug), filepath.FromSlash(rel))
}

// StepOutputRounds returns 1..N where N is the last round before the
// first missing file. Rounds are dense: a gap ends the list. A pattern
// without "{round}" names one file, so it has at most round 1.
func StepOutputRounds(projectRoot, slug, output string) ([]int, error) {
	var rounds []int
	for r := 1; ; r++ {
		_, err := os.Lstat(StepOutputPath(projectRoot, slug, output, r))
		if errors.Is(err, fs.ErrNotExist) {
			return rounds, nil
		}
		if err != nil {
			return nil, err
		}
		rounds = append(rounds, r)
		if !strings.Contains(output, "{round}") {
			return rounds, nil
		}
	}
}

// ReadStepOutput reads and validates round of a step's output and
// returns its verdict ("" when the shape has none). A bad file returns
// an error wrapping ErrInvalidRoleFile.
func ReadStepOutput(projectRoot, slug string, sf StepFiles, round int) (string, error) {
	path := StepOutputPath(projectRoot, slug, sf.Output, round)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	legacy, err := isLegacyFile(projectRoot, slug, relTo(RoleTaskDir(projectRoot, slug), path))
	if err != nil {
		return "", err
	}
	return ParseStepOutput(path, b, sf, legacy)
}

// ParseStepOutput checks b against sf.Shape. Shape "json" needs one
// JSON object with every Required key present and non-null; its
// verdict is a string "verdict" key when there is one.
func ParseStepOutput(path string, b []byte, sf StepFiles, legacy bool) (string, error) {
	switch sf.Shape {
	case "engineer-response":
		_, err := ParseEngineerResponse(path, b, legacy)
		return "", err
	case "verdict":
		r, err := ParseReview(path, b, ReviewerInstance(sf.Author), legacy)
		return r.Verdict, err
	case "json":
		var v any
		if err := decodeRoleJSON(b, &v, true); err != nil {
			return "", invalid(path, err.Error())
		}
		m, ok := v.(map[string]any)
		if !ok {
			return "", invalid(path, "not a JSON object")
		}
		for _, k := range sf.Required {
			if m[k] == nil {
				return "", invalid(path, fmt.Sprintf("required field %q is missing or null", k))
			}
		}
		verdict, _ := m["verdict"].(string)
		return verdict, nil
	}
	return "", invalid(path, fmt.Sprintf("unknown shape %q", sf.Shape))
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
	switch {
	case f.Step != nil:
		_, err = ParseStepOutput(f.Path, b, *f.Step, legacy)
	case f.Instance == "":
		_, err = ParseEngineerResponse(f.Path, b, legacy)
	default:
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
func QuarantinePath(path string) (string, int, error) {
	for n := 1; ; n++ {
		dest := path + InvalidSuffix
		if n > 1 {
			dest = fmt.Sprintf("%s.%d", dest, n)
		}
		_, err := os.Lstat(dest)
		if errors.Is(err, fs.ErrNotExist) {
			return dest, n - 1, nil
		}
		if err != nil {
			return "", 0, fmt.Errorf("quarantine %s: %w", path, err)
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
