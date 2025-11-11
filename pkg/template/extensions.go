package template

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/util"
)

func (tpl *Template) extGit(noSubstitution, path string, defaultValue *string) (string, error) {
	if tpl.git == nil {
		if tpl.strict {
			panic("Strict mode enabled, but Git context wasn't found while processing " + noSubstitution)
		}
		// skip if git is not available
		return noSubstitution, nil
	}
	hash, err := tpl.git.Hash()
	if err != nil {
		return noSubstitution, err
	}
	hashShort := hash[:7]
	if err != nil {
		return noSubstitution, err
	}
	branchRaw, err := tpl.git.Branch()
	if err != nil {
		return noSubstitution, err
	}
	branchClean := strings.ReplaceAll(branchRaw, "/", "-")

	res, err := util.GetValue(path, map[string]interface{}{
		"commit": map[string]string{
			"short": hashShort,
			"full":  hash,
		},
		"branch":       branchClean,
		"branch.raw":   branchRaw,
		"branch.clean": branchClean,
	})
	if err != nil {
		if defaultValue != nil {
			return *defaultValue, err
		}
		return noSubstitution, err
	}
	return res.(string), nil
}

func (tpl *Template) extEnv(noSubstitution, path string, defaultValue *string) (string, error) {
	res := os.Getenv(path)
	if res == "" && defaultValue != nil {
		return *defaultValue, nil
	}
	return res, nil
}

func (tpl *Template) extUser(noSubstitution, path string, defaultValue *string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", errors.Wrapf(err, "failed to detect current user")
	}
	res, err := util.GetValue(path, map[string]interface{}{
		"home":     usr.HomeDir,
		"homeDir":  usr.HomeDir,
		"username": usr.Username,
		"id":       usr.Uid,
		"name":     usr.Name,
	})
	if err != nil {
		if defaultValue != nil {
			return *defaultValue, err
		}
		return noSubstitution, err
	}
	return res.(string), nil
}

func (tpl *Template) extDate(noSubstitution, path string, defaultValue *string) (string, error) {
	var res string
	t := time.Now()
	switch path {
	case "time":
		res = fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
	case "dateOnly":
		res = fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month(), t.Day())
	default:
		if defaultValue != nil {
			return *defaultValue, nil
		}
		return res, errors.Errorf("%s", "unknown date format: "+path)
	}
	return res, nil
}
