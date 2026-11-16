package task

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseReviewStrict(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"ok", `{"author":"A","verdict":"approve","summary":"s","comments":[]}`, ""},
		{"ok no comments", `{"author":"A","verdict":"request_changes","summary":"s"}`, ""},
		{"unknown key", `{"author":"A","verdict":"approve","summary":"s","severity":"high"}`, `unknown field "severity"`},
		{"trailing data", `{"author":"A","verdict":"approve","summary":"s"}{}`, "trailing data"},
		{"empty", ``, "empty file"},
		{"malformed", `{"author":"A",`, "decode"},
		{"missing author", `{"verdict":"approve","summary":"s"}`, `missing required field "author"`},
		{"wrong author", `{"author":"B","verdict":"approve","summary":"s"}`, `author "B" does not match`},
		{"bad verdict", `{"author":"A","verdict":"approved","summary":"s"}`, `verdict "approved"`},
		{"missing summary", `{"author":"A","verdict":"approve"}`, `missing required field "summary"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseReview("x.json", []byte(tc.body), ReviewerA, false)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidRoleFile) {
				t.Fatalf("err = %v, want ErrInvalidRoleFile", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseReviewLegacyTolerance(t *testing.T) {
	if _, err := ParseReview("x", []byte(`{"verdict":"approve","extra":1}`), ReviewerA, true); err != nil {
		t.Errorf("legacy file without author / with extra key: %v", err)
	}
	if _, err := ParseReview("x", []byte(`{"verdict":"lgtm"}`), ReviewerA, true); !errors.Is(err, ErrInvalidRoleFile) {
		t.Errorf("legacy bad verdict err = %v, want invalid", err)
	}
	if _, err := ParseReview("x", []byte(`{"author":"B","verdict":"approve"}`), ReviewerA, true); !errors.Is(err, ErrInvalidRoleFile) {
		t.Errorf("legacy wrong author err = %v, want invalid", err)
	}
}

func TestParseEngineerResponseStrict(t *testing.T) {
	if _, err := ParseEngineerResponse("x", []byte(`{"author":"engineer","addressed":[],"followups":["retry svc"]}`), false); err != nil {
		t.Errorf("valid response: %v", err)
	}
	for _, body := range []string{
		`{"addressed":[]}`,
		`{"author":"A","addressed":[]}`,
		`{"author":"engineer","fixed":[]}`,
		"{\"author\":\"engineer\"}\ncat: -: hung",
	} {
		if _, err := ParseEngineerResponse("x", []byte(body), false); !errors.Is(err, ErrInvalidRoleFile) {
			t.Errorf("body %q: err = %v, want invalid", body, err)
		}
	}
}

func TestEnsureLegacyMarkerGrandfathersExistingFilesOnly(t *testing.T) {
	root := t.TempDir()
	slug := "demo"
	old := ReviewPath(root, slug, ReviewerA, 1)
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(`{"verdict":"request_changes","summary":"s","stray":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReview(root, slug, ReviewerA, 1); err != nil {
		t.Fatalf("pre-marker read should treat file as legacy: %v", err)
	}
	if err := EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReview(root, slug, ReviewerA, 1); err != nil {
		t.Fatalf("grandfathered file rejected: %v", err)
	}

	fresh := ReviewPath(root, slug, ReviewerA, 2)
	if err := os.WriteFile(fresh, []byte(`{"verdict":"approve","summary":"s"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureLegacyMarker(root, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReview(root, slug, ReviewerA, 2); !errors.Is(err, ErrInvalidRoleFile) {
		t.Fatalf("post-marker file without author: err = %v, want invalid", err)
	}
}

func TestQuarantinePathIncrements(t *testing.T) {
	p := filepath.Join(t.TempDir(), "round-1.json")
	dest, prior := QuarantinePath(p)
	if dest != p+".invalid" || prior != 0 {
		t.Fatalf("first = %s, %d", dest, prior)
	}
	if err := os.WriteFile(dest, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	dest, prior = QuarantinePath(p)
	if dest != p+".invalid.2" || prior != 1 {
		t.Fatalf("second = %s, %d", dest, prior)
	}
}

func TestListRoleFilesExpectedAuthors(t *testing.T) {
	root := t.TempDir()
	if err := WriteEngineerResponse(root, "demo", 1, EngineerResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := WriteReview(root, "demo", ReviewerB, 1, Review{Verdict: VerdictApprove, Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	files, err := ListRoleFiles(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %+v", files)
	}
	if files[0].Rel != "responses/engineer-round-1.json" || files[0].Author() != EngineerAuthor {
		t.Errorf("engineer file = %+v", files[0])
	}
	if files[1].Rel != "reviews/B/round-1.json" || files[1].Author() != "B" {
		t.Errorf("review file = %+v", files[1])
	}
	for _, f := range files {
		if err := ValidateRoleFile(root, "demo", f); err != nil {
			t.Errorf("%s: %v", f.Rel, err)
		}
	}
}
