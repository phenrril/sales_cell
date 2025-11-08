package localfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Storage struct{ base string }

func New(base string) *Storage { return &Storage{base: base} }

func (s *Storage) SaveModel(ctx context.Context, filename string, data []byte) (string, error) {
	return s.save(ctx, "models", filename, data)
}

func (s *Storage) SaveImage(ctx context.Context, filename string, data []byte) (string, error) {
	return s.save(ctx, "images", filename, data)
}

func (s *Storage) save(ctx context.Context, sub, filename string, data []byte) (string, error) {
	_ = ctx
	dir := filepath.Join(s.base, sub)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	fname := fmt.Sprintf("%d-%s", time.Now().UnixNano(), filename)
	path := filepath.Join(dir, fname)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}
