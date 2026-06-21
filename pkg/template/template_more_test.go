// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package template

import (
	"os/user"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/util"
)

func TestEvalToBool(t *testing.T) {
	cases := []struct {
		name    string
		expr    string
		want    bool
		wantErr bool
	}{
		{"literal true", "true", true, false},
		{"literal false", "false", false, false},
		{"comparison true", "1 == 1", true, false},
		{"comparison false", "1 == 2", false, false},
		{"compile error", "1 +", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			got, err := NewTemplate().EvalToBool(tc.expr)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(got).To(Equal(tc.want))
		})
	}
}

func TestEvalToBool_WithDataAndPlaceholders(t *testing.T) {
	RegisterTestingT(t)

	tpl := NewTemplate().WithData(util.Data{"enabled": true})
	got, err := tpl.EvalToBool("enabled == true")
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeTrue())
}

func TestExtDate(t *testing.T) {
	RegisterTestingT(t)
	tpl := NewTemplate()

	t.Run("time format", func(t *testing.T) {
		RegisterTestingT(t)
		out := tpl.Exec("${date:time}")
		Expect(out).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`))
	})

	t.Run("dateOnly format", func(t *testing.T) {
		RegisterTestingT(t)
		out := tpl.Exec("${date:dateOnly}")
		Expect(out).To(MatchRegexp(`^\d{4}-\d{2}-\d{2}$`))
	})

	t.Run("unknown format with default", func(t *testing.T) {
		RegisterTestingT(t)
		out := tpl.Exec("${date:bogus:fallback}")
		Expect(out).To(Equal("fallback"))
	})

	t.Run("unknown format without default leaves placeholder (non-strict)", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${date:bogus}")).To(Equal("${date:bogus}"))
	})
}

func TestExtUser(t *testing.T) {
	RegisterTestingT(t)
	cur, err := user.Current()
	Expect(err).ToNot(HaveOccurred())

	tpl := NewTemplate()
	Expect(tpl.Exec("${user:username}")).To(Equal(cur.Username))
	Expect(tpl.Exec("${user:home}")).To(Equal(cur.HomeDir))
	Expect(tpl.Exec("${user:homeDir}")).To(Equal(cur.HomeDir))
	Expect(tpl.Exec("${user:id}")).To(Equal(cur.Uid))

	// Quirk: unlike extDate, extUser returns its default *together with* the
	// lookup error, and calcValue discards the result whenever err != nil — so
	// the default is NOT applied and the raw placeholder survives (non-strict).
	Expect(tpl.Exec("${user:bogus:fallback}")).To(Equal("${user:bogus:fallback}"))
}

func TestExtEnv(t *testing.T) {
	RegisterTestingT(t)
	tpl := NewTemplate()

	t.Run("set variable", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TPL_TEST_VAR", "hello")
		Expect(tpl.Exec("${env:SC_TPL_TEST_VAR}")).To(Equal("hello"))
	})

	t.Run("unset with default", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${env:SC_TPL_DEFINITELY_UNSET:fallback}")).To(Equal("fallback"))
	})
}

func TestExtGit_NilGit(t *testing.T) {
	RegisterTestingT(t)

	t.Run("non-strict leaves placeholder", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := NewTemplate() // no git configured
		Expect(tpl.Exec("${git:commit.short}")).To(Equal("${git:commit.short}"))
	})

	t.Run("strict mode panics", func(t *testing.T) {
		RegisterTestingT(t)
		tpl := NewTemplate().WithStrict(true)
		Expect(func() { tpl.Exec("${git:commit.short}") }).To(Panic())
	})
}

func TestCalcValue_DataPaths(t *testing.T) {
	RegisterTestingT(t)

	tpl := NewTemplate().WithData(util.Data{
		"simple": "value",
		"nested": map[string]interface{}{"key": "deep"},
	})

	t.Run("no-context placeholder from data", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${simple}")).To(Equal("value"))
	})

	t.Run("unknown no-context placeholder is left as-is", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${UNKNOWN}")).To(Equal("${UNKNOWN}"))
	})

	t.Run("context:path traversal into nested data", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${nested:key}")).To(Equal("deep"))
	})

	t.Run("missing context:path with default", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${nested:missing:fallback}")).To(Equal("fallback"))
	})

	t.Run("missing context:path without default left as-is", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(tpl.Exec("${ghost:path}")).To(Equal("${ghost:path}"))
	})
}

func TestCustomExtension(t *testing.T) {
	RegisterTestingT(t)

	tpl := NewTemplate().WithExtensions(map[string]Extension{
		"custom": func(source, path string, def *string) (string, error) {
			return "X-" + path, nil
		},
	})
	Expect(tpl.Exec("${custom:abc}")).To(Equal("X-abc"))
}
