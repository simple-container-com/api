package template

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	gitmocks "github.com/simple-container-com/api/pkg/api/git/mocks"
	"github.com/simple-container-com/api/pkg/util"
)

func TestSafeWhenNoMatch(t *testing.T) {
	RegisterTestingT(t)
	parsedTpl := NewTemplate().WithData(util.Data{"container:home": "/root", "host:wd": "/home/bob"})

	result := parsedTpl.Exec(`Hello, ${SOME_VAR}! Your home: ${container:home}, current dir: ${host:wd}`)
	Expect(result).To(Equal("Hello, ${SOME_VAR}! Your home: /root, current dir: /home/bob"))
}

func TestExtensions(t *testing.T) {
	RegisterTestingT(t)
	parsedTpl := NewTemplate().WithExtensions(map[string]Extension{
		"ext": func(noSubs, path string, defaultVal *string) (string, error) {
			if path == "my.very-custom.path" {
				return "very-custom-value", nil
			}
			if path == "error-path" {
				return "", errors.New("expected error")
			}
			if defaultVal != nil {
				return *defaultVal, nil
			}
			return noSubs, nil
		},
	}).WithStrict(true)

	result := parsedTpl.Exec(`
		Custom path: ${ext:my.very-custom.path}
		Error path: ${ext:error-path}
		Default path: ${ext:non-existing-path:default-value}

	`)
	Expect(result).To(ContainSubstring("Custom path: very-custom-value"))
	Expect(result).To(ContainSubstring("Error path: ${ext:error-path}; error: expected error"))
	Expect(result).To(ContainSubstring("Default path: default-value"))
}

func TestTemplating(t *testing.T) {
	RegisterTestingT(t)
	gitMock := gitmocks.GitRepoMock{}
	gitMock.On("Hash").Return("1234567890f", nil)
	gitMock.On("Branch").Return("feature/micros", nil)

	parsedTpl := NewTemplate().
		WithData(util.Data{
			"container:home":   "/home/bob",
			"host:wd":          "/home/bob",
			"arg:some-arg-key": "some-arg-value",
			"mode:build":       "sox",
			"some":             util.Data{"deep": util.Data{"field": "value"}},
			"user": util.Data{
				"name":     "Vasya",
				"username": "vasya",
				"email":    "vasya@vasya.com",
			},
			"prize": "foobar",
		}).
		WithGit(&gitMock)

	defer os.Setenv("SOME_VAR", "")
	os.Setenv("SOME_VAR", "some value")

	result := parsedTpl.Exec(`Hello, ${user.name}! You won ${prize}!
		Your email: ${user.email}.
		Current time: ${date:time}. Current date: ${date:dateOnly}.
		Some env variable: ${env:SOME_VAR}
		Some invalid token: ${invalid:BLABLA}.
		Some arg: ${arg:some-arg-key}.
		Git hash: ${git:commit.full}.
		Bob's home: ${container:home}.
		Git hash short: ${git:commit.short}.
		Git branch: ${git:branch}.
		Git branch raw: ${git:branch.raw}.
		Git branch clean: ${git:branch.clean}.
		Build mode: ${mode:build}.
		With env empty default: ${env:NOT_EXISTING_ENV:}.
		With env default: ${env:NOT_EXISTING_ENV:default}.
		With env with no default: ${env:NOT_EXISTING_ENV}.
		With arg with empty default: ${arg:not-existing-arg:}.
		With arg with default: ${arg:not-existing-arg:default}.
		With existing arg with default: ${arg:some-arg-key:default}.
		With deep field value: ${some:deep.field:default}.
		Another deep field with default: ${another:deep.field:default}.
		`)

	Expect(result).To(ContainSubstring("Hello, Vasya"))
	Expect(result).To(ContainSubstring("You won foobar"))
	Expect(result).To(ContainSubstring("Your email: vasya@vasya.com"))
	Expect(result).To(MatchRegexp("Current time: [\\d]{4}-[\\d]{2}-[\\d]{2}T[\\d]{2}:[\\d]{2}:[\\d]{2}\\."))
	Expect(result).To(MatchRegexp("Current date: [\\d]{4}-[\\d]{2}-[\\d]{2}\\."))
	Expect(result).To(ContainSubstring("Some env variable: some value"))
	Expect(result).To(ContainSubstring("Some invalid token: ${invalid:BLABLA}."))
	Expect(result).To(ContainSubstring("Some arg: some-arg-value."))
	Expect(result).To(ContainSubstring("Git hash: 1234567890f"))
	Expect(result).To(ContainSubstring("Git hash short: 1234567"))
	Expect(result).To(ContainSubstring("Build mode: sox."))
	Expect(result).To(ContainSubstring("Bob's home: /home/bob."))
	Expect(result).To(ContainSubstring("Git branch: feature-micros"))
	Expect(result).To(ContainSubstring("Git branch raw: feature/micros"))
	Expect(result).To(ContainSubstring("Git branch clean: feature-micros"))
	Expect(result).To(ContainSubstring("With env empty default: ."))
	Expect(result).To(ContainSubstring("With env default: default."))
	Expect(result).To(ContainSubstring("With env with no default: ."))
	Expect(result).To(ContainSubstring("With arg with empty default: ."))
	Expect(result).To(ContainSubstring("With arg with default: default."))
	Expect(result).To(ContainSubstring("With existing arg with default: some-arg-value."))
	Expect(result).To(ContainSubstring("With deep field value: value."))
	Expect(result).To(ContainSubstring("Another deep field with default: default."))
}
