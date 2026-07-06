package runner

import (
	"bytes"
	"os"

	"gopkg.in/yaml.v3"
)

// mergeComments re-serializes newData with comments carried over from oldData
// wherever the two YAML structures line up by map key (and sequence index).
//
// Cluster/helm output never contains comments, so this is how hand-written
// annotations in a committed file survive a refresh: fresh data always wins for
// values, while the old file contributes only its comments. Comments attached to
// a key that no longer exists in the new data have nowhere to land and are
// dropped. If either document fails to parse, newData is returned unchanged.
func mergeComments(oldData, newData []byte) ([]byte, error) {
	var oldNode, newNode yaml.Node
	if err := yaml.Unmarshal(oldData, &oldNode); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(newData, &newNode); err != nil {
		return nil, err
	}
	// Nothing to merge if either side is empty (Kind 0 == undecoded).
	if oldNode.Kind == 0 || newNode.Kind == 0 {
		return newData, nil
	}

	copyComments(&oldNode, &newNode)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // match kubectl-neat / helm 2-space indentation
	if err := enc.Encode(&newNode); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// copyComments copies head/line/foot comments from old onto new, then recurses
// into matching children. Mappings are matched by key, sequences by index.
func copyComments(old, new *yaml.Node) {
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

	switch new.Kind {
	case yaml.DocumentNode:
		if old.Kind == yaml.DocumentNode && len(old.Content) > 0 && len(new.Content) > 0 {
			copyComments(old.Content[0], new.Content[0])
		}
	case yaml.MappingNode:
		if old.Kind != yaml.MappingNode {
			return
		}
		// Index old entries by key so we match on key name, not position —
		// kubectl and helm reorder fields freely between fetches.
		oldByKey := make(map[string][2]*yaml.Node, len(old.Content)/2)
		for i := 0; i+1 < len(old.Content); i += 2 {
			oldByKey[old.Content[i].Value] = [2]*yaml.Node{old.Content[i], old.Content[i+1]}
		}
		for i := 0; i+1 < len(new.Content); i += 2 {
			if pair, ok := oldByKey[new.Content[i].Value]; ok {
				copyComments(pair[0], new.Content[i])   // key node (head/inline comment on the key line)
				copyComments(pair[1], new.Content[i+1]) // value subtree
			}
		}
	case yaml.SequenceNode:
		if old.Kind != yaml.SequenceNode {
			return
		}
		n := len(old.Content)
		if len(new.Content) < n {
			n = len(new.Content)
		}
		for i := 0; i < n; i++ {
			copyComments(old.Content[i], new.Content[i])
		}
	}
}

// mergeCommentsFromFile reads the existing file at path (if any) and returns
// newData with its comments merged in. When the file is missing, empty, or
// unparseable, newData is returned unchanged along with a non-nil error only
// for a genuine merge failure (a missing file is not an error).
func mergeCommentsFromFile(path string, newData []byte) ([]byte, error) {
	existing, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(existing)) == 0 {
		return newData, nil // no existing content to preserve
	}
	return mergeComments(existing, newData)
}
