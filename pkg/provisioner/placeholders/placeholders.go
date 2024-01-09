package placeholders

import (
	"reflect"
	"strings"

	"api/pkg/api/logger"

	"api/pkg/api"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/welder/pkg/template"
	"github.com/simple-container-com/welder/pkg/welder/types"
)

type Placeholders interface {
	// Apply applies placeholders resolve to an object
	Apply(obj any, opts ...Option) error

	// Resolve resolves all placeholders (like ${auth:<something}) in a stack map
	Resolve(stacks api.StacksMap) error
}

type Option func(tpl *template.Template)

type placeholders struct {
	log logger.Logger
}

func WithExtensions(extensions map[string]template.Extension) Option {
	return func(tpl *template.Template) {
		tpl.WithExtensions(extensions)
	}
}

func (p *placeholders) Apply(obj any, opts ...Option) error {
	return p.applyTemplatesOnObject(obj, opts)
}

func (p *placeholders) Resolve(stacks api.StacksMap) error {
	stacks = *stacks.ResolveInheritance()
	iterStacks := lo.Assign(stacks)
	for stackName, stack := range iterStacks {
		opts := []Option{
			WithExtensions(map[string]template.Extension{
				"auth":   p.tplAuth(stackName, stack, stacks),
				"secret": p.tplSecrets(stackName, stack, stacks),
				"var":    p.tplVars(stackName, stack, stacks),
				"stack":  p.tplStack(stackName, stack, stacks),
			}),
		}
		if err := p.Apply(&stack, opts...); err != nil {
			return err
		}
		stacks[stackName] = stack
	}
	return nil
}

func (p *placeholders) tplStack(stackName string, stack api.Stack, stacks api.StacksMap) func(source string, path string, value *string) (string, error) {
	return func(noSubs, path string, value *string) (string, error) {
		vars := map[string]string{
			"name": stackName,
		}
		if val, found := vars[path]; !found {
			return noSubs, errors.Errorf("value %q not found for stack %q", path, stackName)
		} else {
			return val, nil
		}
	}
}

func (p *placeholders) tplVars(stackName string, stack api.Stack, stacks api.StacksMap) func(source string, path string, value *string) (string, error) {
	return func(noSubs, path string, value *string) (string, error) {
		if val, found := stack.Server.Variables[path]; !found {
			return noSubs, errors.Errorf("variable %q not found for stack %q", path, stackName)
		} else {
			return val.Value, nil
		}
	}
}

func (p *placeholders) tplSecrets(stackName string, stack api.Stack, stacks api.StacksMap) func(source string, path string, value *string) (string, error) {
	return func(noSubs, path string, value *string) (string, error) {
		if stack.Server.Secrets.IsInherited() {
			parentStack := stack.Server.Secrets.Inherit.Inherit
			if iServerCfg, ok := stacks[parentStack]; !ok {
				return noSubs, errors.Errorf("parent stack %q not found for stack %q", parentStack, stackName)
			} else if sec, ok := iServerCfg.Secrets.Values[path]; !ok {
				return noSubs, errors.Errorf("secret %q not found in parent stack %q", path, parentStack)
			} else {
				return sec, nil
			}
		} else if sec, ok := stack.Secrets.Values[path]; !ok {
			return noSubs, errors.Errorf("secret %q not found in stack %q", path, stackName)
		} else {
			return sec, nil
		}
	}
}

func (p *placeholders) tplAuth(stackName string, stack api.Stack, stacks api.StacksMap) func(source string, path string, value *string) (string, error) {
	return func(noSubs, path string, value *string) (string, error) {
		pathParts := strings.SplitN(path, ".", 2)
		var resAuth api.AuthDescriptor
		if auth, ok := stack.Secrets.Auth[pathParts[0]]; !ok {
			return noSubs, errors.Errorf("auth %s not found in stack %s", pathParts[0], stackName)
		} else if !auth.IsInherited() {
			resAuth = auth
		} else if pAuth, ok := stacks[auth.Inherit.Inherit].Secrets.Auth[pathParts[0]]; auth.IsInherited() && ok {
			resAuth = pAuth
		}

		if resAuth.Type != "" {
			authConfig, err := resAuth.AuthConfig()
			if err != nil {
				return noSubs, err
			}
			if len(pathParts) == 1 {
				return authConfig.CredentialsValue(), nil
			} else if len(pathParts) == 2 {
				if res, ok := map[string]string{
					"projectId":   authConfig.ProjectIdValue(),
					"credentials": authConfig.CredentialsValue(),
				}[pathParts[1]]; ok {
					return res, nil
				}
			}
		}
		return noSubs, errors.Errorf("inherited auth %s not found in stack %s", path, stackName)
	}
}

func New(log logger.Logger) Placeholders {
	return &placeholders{
		log: log,
	}
}

func (p *placeholders) initTemplate(opts []Option) *template.Template {
	tpl := template.NewTemplate()
	for _, opt := range opts {
		opt(tpl)
	}
	return tpl
}

// value must be a string
func (p *placeholders) applyTemplateOnString(value string, opts []Option) string {
	return p.initTemplate(opts).Exec(value)
}

// out must be a pointer
func (p *placeholders) applyTemplatesOnObject(out any, opts []Option) error {
	rv := reflect.ValueOf(out)
	reflectedVal := rv.Elem()
	appliedResult := p.applyTemplates(out, opts)
	val := reflect.ValueOf(appliedResult).Elem()
	reflectedVal.Set(val)
	return nil
}

func (p *placeholders) applyTemplates(obj any, opts []Option) any {
	// Wrap the original in a reflect.Value
	original := reflect.ValueOf(obj)
	res := reflect.New(original.Type()).Elem()
	p.applyTemplatesRecursive(res, original, opts)
	// Remove the reflection wrapper
	return res.Interface()
}

func (p *placeholders) applyTemplatesRecursive(copy, original reflect.Value, opts []Option) {
	switch original.Kind() {
	// The first cases handle nested structures and translate them recursively

	// If it is a pointer we need to unwrap and call once again
	case reflect.Ptr:
		// To get the actual value of the original we have to call Elem()
		// At the same time this unwraps the pointer so we don't end up in
		// an infinite recursion
		originalValue := original.Elem()
		// Check if the pointer is nil
		if !originalValue.IsValid() {
			return
		}
		// Allocate a new object and set the pointer to it
		copy.Set(reflect.New(originalValue.Type()))
		// Unwrap the newly created pointer
		p.applyTemplatesRecursive(copy.Elem(), originalValue, opts)

	// If it is an interface (which is very similar to a pointer), do basically the
	// same as for the pointer. Though a pointer is not the same as an interface so
	// note that we have to call Elem() after creating a new object because otherwise
	// we would end up with an actual pointer
	case reflect.Interface:
		// Get rid of the wrapping interface
		originalValue := original.Elem()

		// Create a new object. Now new gives us a pointer, but we want the value it
		// points to, so we have to call Elem() to unwrap it
		if originalValue.IsValid() && copy.CanSet() {
			copyValue := reflect.New(originalValue.Type()).Elem()
			p.applyTemplatesRecursive(copyValue, originalValue, opts)
			copy.Set(copyValue)
		}

	// If it is a struct we translate each field
	case reflect.Struct:
		for i := 0; i < original.NumField(); i += 1 {
			p.applyTemplatesRecursive(copy.Field(i), original.Field(i), opts)
		}

	// If it is a slice we create a new slice and translate each element
	case reflect.Slice:
		copy.Set(reflect.MakeSlice(original.Type(), original.Len(), original.Cap()))
		for i := 0; i < original.Len(); i += 1 {
			p.applyTemplatesRecursive(copy.Index(i), original.Index(i), opts)
		}

	// If it is a map we create a new map and translate each value
	case reflect.Map:
		copy.Set(reflect.MakeMap(original.Type()))
		for _, key := range original.MapKeys() {
			originalValue := original.MapIndex(key)
			// New gives us a pointer, but again we want the value
			copyValue := reflect.New(originalValue.Type()).Elem()
			p.applyTemplatesRecursive(copyValue, originalValue, opts)
			copy.SetMapIndex(key, copyValue)
		}

	// Otherwise we cannot traverse anywhere so this finishes the recursion

	// If it is a string translate it (yay finally we're doing what we came for)
	case reflect.String:
		var processed string
		originalVal := original.Interface()
		if _, ok := originalVal.(string); ok {
			processed = p.applyTemplateOnString(originalVal.(string), opts)
		} else if _, ok := originalVal.(types.StringValue); ok {
			processed = p.applyTemplateOnString(string(originalVal.(types.StringValue)), opts)
		} else {
			processed = p.applyTemplateOnString(string(originalVal.(types.RunOnType)), opts)
		}
		copy.SetString(processed)

	// And everything else will simply be taken from the original
	default:
		copy.Set(original)
	}
}
