// Package testfixture locates external validation inputs. Only tests import it.
package testfixture

import (
	"os"
	"path/filepath"
	"strings"
)

// Path resolves named fixture inputs, including directory prefixes. An absent
// root leaves the path unresolved inside the test package, never in a live store.
func Path(key string) string {
	root := os.Getenv("GS2000_FIXTURE_ROOT")
	if root == "" {
		root = ".external-fixtures"
	}
	path := filepath.Join(root, filepath.FromSlash(key))
	if strings.HasSuffix(key, "/") {
		path += string(filepath.Separator)
	}
	return path
}
