package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadContextMap reads a .context-map file and returns a map of
// directory-name -> kubectl-context-name.
// Returns an empty map (no error) if the file does not exist.
func LoadContextMap(baseDir string) (map[string]string, error) {
	path := filepath.Join(baseDir, ".context-map")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return m, scanner.Err()
}
