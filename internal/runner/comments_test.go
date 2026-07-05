package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeComments(t *testing.T) {
	old := []byte(`# top-of-file note about this deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web        # inline note on name
  labels:
    # head comment on a nested key
    team: payments
spec:
  replicas: 3      # bumped for Black Friday
  gone: true       # this key is removed in the new data
`)
	// Fresh cluster output: no comments, replicas changed, a key added and removed.
	new := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    team: payments
spec:
  replicas: 5
  added: hello
`)

	got, err := mergeComments(old, new)
	if err != nil {
		t.Fatalf("mergeComments: %v", err)
	}
	out := string(got)

	wantContains := []string{
		"# top-of-file note about this deployment",
		"name: web # inline note on name",
		"# head comment on a nested key",
		"replicas: 5 # bumped for Black Friday", // comment survives a value change
		"added: hello",                          // new key present
	}
	for _, w := range wantContains {
		if !strings.Contains(out, w) {
			t.Errorf("merged output missing %q\n--- got ---\n%s", w, out)
		}
	}

	// A comment attached to a removed key must not linger.
	if strings.Contains(out, "this key is removed") {
		t.Errorf("comment for removed key leaked into output\n--- got ---\n%s", out)
	}
	// Value must come from the new data, not the old.
	if strings.Contains(out, "replicas: 3") {
		t.Errorf("stale value replicas: 3 present\n--- got ---\n%s", out)
	}
}

func TestMergeCommentsUnparseableOldFallsBack(t *testing.T) {
	new := []byte("kind: Deployment\n")
	_, err := mergeComments([]byte(":\n  : not: valid: yaml"), new)
	if err == nil {
		t.Fatal("expected parse error for malformed old data")
	}
	// FetchAndSave treats this error as non-fatal and writes new as-is.
}

func TestMergeCommentsFromFileMissing(t *testing.T) {
	new := []byte("kind: Deployment\n")
	got, err := mergeCommentsFromFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"), new)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if string(got) != string(new) {
		t.Errorf("missing file should return new data unchanged, got %q", got)
	}
}

func TestMergeCommentsFromFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dep.yaml")
	if err := os.WriteFile(path, []byte("kind: Deployment # keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := mergeCommentsFromFile(path, []byte("kind: Deployment\n"))
	if err != nil {
		t.Fatalf("mergeCommentsFromFile: %v", err)
	}
	if !strings.Contains(string(got), "# keep me") {
		t.Errorf("comment not preserved from existing file, got %q", got)
	}
}
