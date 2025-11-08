//go:build ignore
// +build ignore

package main

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	var files []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".go") {

			if strings.Contains(path, string(filepath.Separator)+"vendor"+string(filepath.Separator)) {
				return nil
			}
			files = append(files, path)
		}
		return nil
	})
	for _, f := range files {
		if err := process(f); err != nil {
			log.Printf("error %s: %v", f, err)
		}
	}
}

func process(path string) error {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	filtered := []*ast.CommentGroup{}
	for _, cg := range file.Comments {
		keep := false
		for _, c := range cg.List {
			text := c.Text
			if strings.HasPrefix(text, "//go:") {
				keep = true
				break
			}
		}
		if keep {
			filtered = append(filtered, cg)
		}
	}
	file.Comments = filtered

	var out strings.Builder
	if err := format.Node(&out, set, file); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out.String()), 0644)
}
