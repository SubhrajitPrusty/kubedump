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

	got, dropped, err := mergeComments(old, new)
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
	// ...but it must be reported rather than silently lost.
	if len(dropped) != 1 || dropped[0].Path != "spec.gone" || dropped[0].Lines != 1 {
		t.Errorf("want one dropped comment at spec.gone (1 line), got %+v", dropped)
	}
}

// The case that motivated reporting: a hand-added Helm value whose key the live
// release never had, so a multi-line rationale vanishes on every refresh.
func TestMergeCommentsReportsDroppedHandAddedKey(t *testing.T) {
	old := []byte(`datadog:
  logs:
    enabled: false
  # The operator subchart reconciles nothing here — the agent is installed by
  # this chart directly, not via a DatadogAgent CR, and no DatadogAgent CRs
  # exist.
  operator:
    enabled: false
  orchestratorExplorer:
    enabled: true
`)
	new := []byte(`datadog:
  logs:
    enabled: false
  orchestratorExplorer:
    enabled: true
`)

	got, dropped, err := mergeComments(old, new)
	if err != nil {
		t.Fatalf("mergeComments: %v", err)
	}
	if strings.Contains(string(got), "operator") {
		t.Errorf("dump should still reflect live state\n--- got ---\n%s", got)
	}
	if len(dropped) != 1 {
		t.Fatalf("want 1 dropped comment, got %+v", dropped)
	}
	if dropped[0].Path != "datadog.operator" {
		t.Errorf("want path datadog.operator, got %q", dropped[0].Path)
	}
	if dropped[0].Lines != 3 {
		t.Errorf("want 3 comment lines counted, got %d", dropped[0].Lines)
	}
}

// A key vanishing without any comment is the dump working normally, not a loss.
func TestMergeCommentsIgnoresUncommentedRemovals(t *testing.T) {
	old := []byte("spec:\n  replicas: 3\n  gone: true\n")
	new := []byte("spec:\n  replicas: 3\n")

	_, dropped, err := mergeComments(old, new)
	if err != nil {
		t.Fatalf("mergeComments: %v", err)
	}
	if len(dropped) != 0 {
		t.Errorf("uncommented removal should not be reported, got %+v", dropped)
	}
}

func TestMergeCommentsReportsTruncatedSequenceItem(t *testing.T) {
	old := []byte("args:\n  - --a\n  - --b # why b is here\n")
	new := []byte("args:\n  - --a\n")

	_, dropped, err := mergeComments(old, new)
	if err != nil {
		t.Fatalf("mergeComments: %v", err)
	}
	if len(dropped) != 1 || dropped[0].Path != "args[1]" {
		t.Errorf("want one dropped comment at args[1], got %+v", dropped)
	}
}

func TestMergeCommentsUnparseableOldFallsBack(t *testing.T) {
	new := []byte("kind: Deployment\n")
	_, _, err := mergeComments([]byte(":\n  : not: valid: yaml"), new)
	if err == nil {
		t.Fatal("expected parse error for malformed old data")
	}
	// FetchAndSave treats this error as non-fatal and writes new as-is.
}

func TestMergeCommentsFromFileMissing(t *testing.T) {
	new := []byte("kind: Deployment\n")
	got, dropped, err := mergeCommentsFromFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"), new)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if string(got) != string(new) {
		t.Errorf("missing file should return new data unchanged, got %q", got)
	}
	if len(dropped) != 0 {
		t.Errorf("missing file should report nothing dropped, got %+v", dropped)
	}
}

func TestMergeCommentsFromFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dep.yaml")
	if err := os.WriteFile(path, []byte("kind: Deployment # keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _, err := mergeCommentsFromFile(path, []byte("kind: Deployment\n"))
	if err != nil {
		t.Fatalf("mergeCommentsFromFile: %v", err)
	}
	if !strings.Contains(string(got), "# keep me") {
		t.Errorf("comment not preserved from existing file, got %q", got)
	}
}
