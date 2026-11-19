package evalharness

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// qualityRules are the mechanical code-quality checks on the source
// files a qa-author run added or changed, in report order.
var qualityRules = []string{"no-added-comments", "no-visibility-only-asserts", "no-duplicate-helpers", "no-api-end-state"}

var sourceExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}

var (
	commentLine  = regexp.MustCompile(`^\s*(//|/\*|\*)`)
	testStart    = regexp.MustCompile(`^\s*test(\.only|\.skip|\.fixme)?\(`)
	visibilityAs = regexp.MustCompile(`\.(not\.)?(toBeVisible|toBeAttached|toBeInViewport|toBeEnabled|toBeDefined|toBeTruthy)\(\s*\)`)
	apiEndState  = regexp.MustCompile(`expect\(\s*(await\s+)?[\w.]*\b(response|resp|res|apiResponse)\b|expect\(.*\.(status|json|ok)\(\)`)
	helperDefs   = []*regexp.Regexp{
		regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)\s*\(`),
		regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?(?:function\b|\([^)]*\)\s*(?::[^=]*)?=>|[A-Za-z_$][\w$]*\s*=>)`),
		regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async)\s+)*([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*(?::\s*[^{]+)?\{\s*$`),
	}
	notHelpers = []string{"if", "for", "while", "switch", "catch", "function", "return", "test", "describe", "constructor"}
)

type sourceFile struct {
	rel   string
	lines []string
	added map[int]bool
}

// qualityFindings names each rule in rules (all when empty) that a file
// the agent added or changed under runWD breaks, against snapshot.
func qualityFindings(snapshot, runWD string, rules []string) ([]string, error) {
	if len(rules) == 0 {
		rules = qualityRules
	}
	changed, err := changedSources(snapshot, runWD)
	if err != nil {
		return nil, err
	}
	known, err := snapshotHelpers(snapshot)
	if err != nil {
		return nil, err
	}
	broken := map[string]bool{}
	for _, f := range changed {
		for i, line := range f.lines {
			if !f.added[i] {
				continue
			}
			if commentLine.MatchString(line) {
				broken["no-added-comments"] = true
			}
			if apiEndState.MatchString(line) {
				broken["no-api-end-state"] = true
			}
			if name := helperName(line); name != "" && known[name] {
				broken["no-duplicate-helpers"] = true
			}
		}
		if visibilityOnlyTest(f) {
			broken["no-visibility-only-asserts"] = true
		}
	}
	var out []string
	for _, r := range qualityRules {
		if broken[r] && slices.Contains(rules, r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// visibilityOnlyTest reports a test block holding an added line whose
// every assertion only checks that an element shows.
func visibilityOnlyTest(f sourceFile) bool {
	var starts []int
	for i, line := range f.lines {
		if testStart.MatchString(line) {
			starts = append(starts, i)
		}
	}
	for k, start := range starts {
		end := len(f.lines)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		touched, asserts, visible := false, 0, 0
		for i := start; i < end; i++ {
			touched = touched || f.added[i]
			if strings.Contains(f.lines[i], "expect(") {
				asserts++
				if visibilityAs.MatchString(f.lines[i]) {
					visible++
				}
			}
		}
		if touched && asserts > 0 && asserts == visible {
			return true
		}
	}
	return false
}

func helperName(line string) string {
	for _, re := range helperDefs {
		if m := re.FindStringSubmatch(line); m != nil && !slices.Contains(notHelpers, m[1]) {
			return m[1]
		}
	}
	return ""
}

func isSource(path string) bool { return slices.Contains(sourceExts, filepath.Ext(path)) }

func snapshotHelpers(snapshot string) (map[string]bool, error) {
	known := map[string]bool{}
	if !dirExists(snapshot) {
		return known, nil
	}
	err := filepath.WalkDir(snapshot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isSource(path) {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			if name := helperName(line); name != "" {
				known[name] = true
			}
		}
		return nil
	})
	return known, err
}

// changedSources lists the source files under runWD that are new or
// differ from snapshot, each line marked added unless the snapshot's
// version of the file held it.
func changedSources(snapshot, runWD string) ([]sourceFile, error) {
	var out []sourceFile
	err := filepath.WalkDir(runWD, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(runWD, path)
		if d.IsDir() {
			if rel == ".claude" || rel == ".spore" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSource(path) {
			return nil
		}
		now, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		was, _ := os.ReadFile(filepath.Join(snapshot, rel))
		if string(was) == string(now) {
			return nil
		}
		old := map[string]int{}
		for _, l := range strings.Split(string(was), "\n") {
			old[l]++
		}
		f := sourceFile{rel: rel, lines: strings.Split(string(now), "\n"), added: map[int]bool{}}
		for i, l := range f.lines {
			if old[l] > 0 {
				old[l]--
			} else {
				f.added[i] = true
			}
		}
		out = append(out, f)
		return nil
	})
	return out, err
}
