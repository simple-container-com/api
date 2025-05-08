package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Reader interface {
	ReadFile(path string) ([]byte, error)
}

type fileSystemReader struct{}

func (r *fileSystemReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

var FSReader = &fileSystemReader{}

type InlineConfigReader struct {
	WorkDir string
	Configs map[string]string
}

func (r *InlineConfigReader) ReadFile(path string) ([]byte, error) {
	path = strings.TrimPrefix(path, fmt.Sprintf("%s%c", r.WorkDir, filepath.Separator))
	if val, ok := r.Configs[path]; ok {
		return []byte(val), nil
	} else {
		return nil, os.ErrNotExist
	}
}
