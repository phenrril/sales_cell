//go:build ignore
// +build ignore

package main

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// HTML comments <!-- ... --> (no greedy)
	htmlCommentRE = regexp.MustCompile(`<!--(?s:.*?)?-->`)
	// CSS comments /* ... */ (handles nested a bit loosely, non-greedy)
	cssCommentRE = regexp.MustCompile(`/\*[^*]*\*+(?:[^/*][^*]*\*+)*/`)
)

func main() {
	var files []string
	filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".css") {
			files = append(files, path)
		}
		return nil
	})
	for _, f := range files {
		if err := stripFile(f); err != nil {
			log.Printf("error %s: %v", f, err)
		}
	}
}

func stripFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	orig := string(b)
	var out string
	if strings.HasSuffix(path, ".html") {
		out = htmlCommentRE.ReplaceAllString(orig, "")
	} else {
		out = cssCommentRE.ReplaceAllString(orig, "")
	}
	// Trim trailing whitespace lines introduced
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	out = strings.Join(lines, "\n")
	if out == orig {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0644)
}
