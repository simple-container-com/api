package testutil

import (
	"os"

	. "github.com/onsi/gomega"
	"github.com/otiai10/copy"
)

func CopyTempProject(pathToExample string) (string, func(), error) {
	if depDir, err := os.MkdirTemp(os.TempDir(), "project"); err != nil {
		return pathToExample, func() {}, err
	} else if err = copy.Copy(pathToExample, depDir); err != nil {
		return pathToExample, func() {}, err
	} else {
		return depDir, func() { _ = os.RemoveAll(depDir) }, nil
	}
}

func CheckError(err error, checkErr string) {
	if checkErr != "" && err != nil {
		Expect(err.Error()).To(MatchRegexp(checkErr))
	}
}
