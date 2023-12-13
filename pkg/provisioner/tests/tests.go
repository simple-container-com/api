package tests

import (
	"os"

	"api/pkg/api/tests"
	"api/pkg/provisioner/models"

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

var CommonStack = models.Stack{
	Name:    "common",
	Secrets: *tests.CommonSecretsDescriptor,
	Server:  *tests.CommonServerDescriptor,
}

var RefappStack = models.Stack{
	Name:    "refapp",
	Secrets: *tests.CommonSecretsDescriptor,
	Server:  *tests.RefappServerDescriptor,
	Client:  *tests.RefappClientDescriptor,
}
