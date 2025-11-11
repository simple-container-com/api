package template

import (
	"fmt"
	"io"
	"strings"

	"github.com/valyala/fasttemplate"

	"github.com/simple-container-com/api/pkg/api/git"
	"github.com/simple-container-com/api/pkg/util"
)

// Extension allows to extend template engine
type Extension func(source string, path string, defaultValue *string) (string, error)

// Template defines structure for template engine
type Template struct {
	git               git.Repo
	data              util.Data
	strict            bool
	extensions        map[string]Extension
	defaultExtensions map[string]Extension
}

// NewTemplate initializes template engine
func NewTemplate() *Template {
	res := &Template{}
	res.defaultExtensions = map[string]Extension{
		"git":  res.extGit,
		"env":  res.extEnv,
		"date": res.extDate,
		"user": res.extUser,
	}
	return res
}

// Strict sets strict mode
func (tpl *Template) WithStrict(strictMode bool) *Template {
	tpl.strict = strictMode
	return tpl
}

// WithExtensions sets extensions
func (tpl *Template) WithExtensions(extensions map[string]Extension) *Template {
	tpl.extensions = extensions
	return tpl
}

// WithGit sets git object
func (tpl *Template) WithGit(gitObject git.Repo) *Template {
	tpl.git = gitObject
	return tpl
}

// WithData sets extra data for templates
func (tpl *Template) WithData(data util.Data) *Template {
	tpl.data = data
	return tpl
}

// Exec applies template engine to a string with placeholders
func (tpl *Template) Exec(tplString string) string {
	return fasttemplate.ExecuteFuncString(tplString, "${", "}", func(w io.Writer, tag string) (int, error) {
		return w.Write([]byte(tpl.calcValue(tag)))
	})
}

func (tpl *Template) calcValue(tag string) string {
	noSubstitution := fmt.Sprintf("${%s}", tag)
	parts := strings.SplitN(tag, ":", 3)
	context := parts[0]

	// if there is no context specified
	if len(parts[0]) == len(tag) {
		res, err := util.GetValue(tag, tpl.data)
		if err != nil {
			// ignore errors here to ignore placeholders like ${USER}
			return noSubstitution
		}
		return res.(string)
	}

	// if there was context specified
	path := parts[1]
	var defaultValue *string
	if len(parts) > 2 {
		defaultValue = &parts[2]
	}
	// check extra extensions first (if registered)
	if extension, extraExtensionExists := tpl.extensions[context]; extraExtensionExists {
		if res, err := extension(noSubstitution, path, defaultValue); err == nil {
			return res
		} else if tpl.strict {
			return noSubstitution + "; error: " + err.Error()
		} else {
			return noSubstitution
		}
	}
	// check default extensions
	if extension, defaultExtensionExists := tpl.defaultExtensions[context]; defaultExtensionExists {
		if res, err := extension(noSubstitution, path, defaultValue); err == nil {
			return res
		} else if tpl.strict {
			return noSubstitution + "; error: " + err.Error()
		} else {
			return noSubstitution
		}
	}

	// try to traverse path in different ways
	// first trying to check "context:path" string within data
	res, err := util.GetValue(fmt.Sprintf("%s:%s", context, path), tpl.data)
	if err != nil {
		// now trying to check "context": "path"
		if res, err = util.GetValue(context, tpl.data); err == nil {
			if res, err = util.GetValue(path, res); err == nil {
				return res.(string)
			}
		}
		// if nothing is found, return default if present
		if defaultValue != nil {
			return *defaultValue
		}
		// if no default - return without substitution
		return noSubstitution
	}
	return res.(string)
}
