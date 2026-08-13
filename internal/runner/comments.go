package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// droppedComment records a commented key the committed file has but the fresh
// cluster output does not, so its comments had nowhere to land.
type droppedComment struct {
	Path  string // dotted path, e.g. datadog.operator
	Lines int    // comment lines lost
}

// mergeComments re-serializes newData with comments carried over from oldData
// wherever the two YAML structures line up by map key (and sequence index).
//
// Cluster/helm output never contains comments, so this is how hand-written
// annotations in a committed file survive a refresh: fresh data always wins for
// values, while the old file contributes only its comments. Comments attached to
// a key that no longer exists in the new data have nowhere to land and are
// dropped; those are returned so the caller can warn instead of losing a
// hand-written rationale silently. If either document fails to parse, newData is
// returned unchanged.
func mergeComments(oldData, newData []byte) ([]byte, []droppedComment, error) {
	var oldNode, newNode yaml.Node
	if err := yaml.Unmarshal(oldData, &oldNode); err != nil {
		return nil, nil, err
	}
	if err := yaml.Unmarshal(newData, &newNode); err != nil {
		return nil, nil, err
	}
	// Nothing to merge if either side is empty (Kind 0 == undecoded).
	if oldNode.Kind == 0 || newNode.Kind == 0 {
		return newData, nil, nil
	}

	var dropped []droppedComment
	copyComments(&oldNode, &newNode, "", &dropped)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // match kubectl-neat / helm 2-space indentation
	if err := enc.Encode(&newNode); err != nil {
		return nil, nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), dropped, nil
}

// copyComments copies head/line/foot comments from old onto new, then recurses
// into matching children. Mappings are matched by key, sequences by index. Old
// entries with no counterpart in new — a vanished key, a truncated sequence, or
// a value whose shape changed — are recorded in dropped when they carry comments.
func copyComments(old, new *yaml.Node, path string, dropped *[]droppedComment) {
	if old == nil || new == nil {
		return
	}

	if old.HeadComment != "" {
		new.HeadComment = old.HeadComment
	}
	if old.LineComment != "" {
		new.LineComment = old.LineComment
	}
	if old.FootComment != "" {
		new.FootComment = old.FootComment
	}

	// A shape change — mapping → scalar, sequence → mapping, a whole values
	// document collapsing to null — leaves everything nested below with nowhere
	// to land. Report it instead of dropping the subtree silently.
	if old.Kind != new.Kind {
		recordSubtreeDropped(old, path, dropped)
		return
	}

	switch new.Kind {
	case yaml.DocumentNode:
		if len(old.Content) > 0 && len(new.Content) > 0 {
			copyComments(old.Content[0], new.Content[0], path, dropped)
		}
	case yaml.MappingNode:
		// Index new entries by key so we match on key name, not position —
		// kubectl and helm reorder fields freely between fetches. Walking old
		// (not new) is what makes a vanished key observable.
		newByKey := make(map[string]int, len(new.Content)/2)
		for i := 0; i+1 < len(new.Content); i += 2 {
			newByKey[new.Content[i].Value] = i
		}
		for i := 0; i+1 < len(old.Content); i += 2 {
			child := childPath(path, old.Content[i].Value)
			j, ok := newByKey[old.Content[i].Value]
			if !ok {
				recordDropped(child, dropped, old.Content[i], old.Content[i+1])
				continue
			}
			copyComments(old.Content[i], new.Content[j], child, dropped)     // key node (head/inline comment on the key line)
			copyComments(old.Content[i+1], new.Content[j+1], child, dropped) // value subtree
		}
	case yaml.SequenceNode:
		for i, item := range old.Content {
			child := fmt.Sprintf("%s[%d]", path, i)
			if i >= len(new.Content) {
				recordDropped(child, dropped, item)
				continue
			}
			copyComments(item, new.Content[i], child, dropped)
		}
	}
}

// recordSubtreeDropped reports each commented child of old, for when the fresh
// value has a different shape so nothing beneath it can be matched. Comments on
// old itself are not reported — those land on the replacement node.
func recordSubtreeDropped(old *yaml.Node, path string, dropped *[]droppedComment) {
	switch old.Kind {
	case yaml.DocumentNode:
		for _, child := range old.Content {
			recordSubtreeDropped(child, path, dropped)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(old.Content); i += 2 {
			recordDropped(childPath(path, old.Content[i].Value), dropped, old.Content[i], old.Content[i+1])
		}
	case yaml.SequenceNode:
		for i, item := range old.Content {
			recordDropped(fmt.Sprintf("%s[%d]", path, i), dropped, item)
		}
	}
}

// recordDropped appends a droppedComment for path when any of the given nodes
// carries a comment. Uncommented keys vanish all the time — that is the dump
// working — so only commented ones are worth reporting.
func recordDropped(path string, dropped *[]droppedComment, nodes ...*yaml.Node) {
	total := 0
	for _, n := range nodes {
		total += countCommentLines(n)
	}
	if total > 0 {
		*dropped = append(*dropped, droppedComment{Path: path, Lines: total})
	}
}

// countCommentLines totals the comment lines on a node and its descendants.
func countCommentLines(n *yaml.Node) int {
	if n == nil {
		return 0
	}
	total := 0
	for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
		if c == "" {
			continue
		}
		total += len(strings.Split(strings.TrimRight(c, "\n"), "\n"))
	}
	for _, child := range n.Content {
		total += countCommentLines(child)
	}
	return total
}

func childPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// mergeCommentsFromFile reads the existing file at path (if any) and returns
// newData with its comments merged in. A missing or empty file is not an error —
// there is simply nothing to preserve. Any other read failure is returned, since
// a file that exists but cannot be read is about to be overwritten and would
// otherwise lose its comments silently.
func mergeCommentsFromFile(path string, newData []byte) ([]byte, []droppedComment, error) {
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return newData, nil, nil // nothing committed yet
	case err != nil:
		return newData, nil, err
	case len(bytes.TrimSpace(existing)) == 0:
		return newData, nil, nil
	}
	return mergeComments(existing, newData)
}

// warnDroppedComments reports hand-written comments lost because the key they
// annotated is gone from the live state. The dump stays faithful; the loss just
// stops being silent.
func warnDroppedComments(dropped []droppedComment) {
	for _, d := range dropped {
		noun := "lines"
		if d.Lines == 1 {
			noun = "line"
		}
		fmt.Fprintf(os.Stderr, "  [warn] dropped commented key: %s\n", d.Path)
		fmt.Fprintf(os.Stderr, "         (%d comment %s lost; key absent from live state)\n", d.Lines, noun)
	}
}
