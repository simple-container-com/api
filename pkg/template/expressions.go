package template

import (
	"github.com/antonmedv/expr"
	"github.com/pkg/errors"
)

// EvalToBool evaluates expression and converts it to boolean
func (tpl *Template) EvalToBool(tplString string) (bool, error) {
	processed := tpl.Exec(tplString)
	program, err := expr.Compile(processed, expr.Env(tpl.data))
	if err != nil {
		return false, errors.Wrapf(err, "failed to compile bool, source: '%s', processed: '%s'", tplString, processed)
	}
	output, err := expr.Run(program, tpl.data)
	if err != nil {
		return false, errors.Wrapf(err, "failed to eval to bool, source: '%s', processed: '%s'", tplString, processed)
	}
	return output.(bool), nil
}
