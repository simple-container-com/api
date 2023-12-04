package git

import (
	"github.com/go-git/go-billy/v5"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"io"
	"os"
	"strings"
)

func (r *repo) RemoveFileFromIgnore(filePath string) error {
	currentContent, file, err := r.readIgnore(0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	newContent := lo.Filter(strings.Split(string(currentContent), "\n"), func(s string, _ int) bool {
		return s != filePath
	})

	_, file, err = r.readIgnore(os.O_TRUNC)
	if err != nil {
		return err
	}

	_, err = io.WriteString(file, strings.Join(newContent, "\n"))
	if err != nil {
		return errors.Wrapf(err, "failed to write .gitignore file")
	}
	return nil
}

func (r *repo) AddFileToIgnore(filePath string) error {
	currentContent, file, err := r.readIgnore(0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if !strings.Contains(string(currentContent), "\n"+filePath) {
		_, file, err = r.readIgnore(os.O_TRUNC)
		if err != nil {
			return err
		}
		_, err = io.WriteString(file, string(currentContent)+"\n"+filePath)
		if err != nil {
			return errors.Wrapf(err, "failed to write .gitignore file")
		}
	}
	return nil
}

func (r *repo) readIgnore(flag int) ([]byte, billy.File, error) {
	filename := ".gitignore"
	var file billy.File
	var err error
	if _, err := r.wdFs.Stat(filename); os.IsNotExist(err) {
		file, err = r.wdFs.Create(filename)
	} else if err == nil {
		file, err = r.wdFs.OpenFile(filename, os.O_CREATE|os.O_RDWR|flag, os.ModePerm)
	}

	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to open .gitignore file")
	}

	currentContent, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read .gitignore file")
	}

	return currentContent, file, nil
}
