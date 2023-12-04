package tests

import (
	"os"

	"github.com/otiai10/copy"
)

func CopyTempProject(pathToExample string) (string, error) {
	if depDir, err := os.MkdirTemp(os.TempDir(), "project"); err != nil {
		return pathToExample, err
	} else if err = copy.Copy(pathToExample, depDir); err != nil {
		return pathToExample, err
	} else {
		return depDir, nil
	}
}
