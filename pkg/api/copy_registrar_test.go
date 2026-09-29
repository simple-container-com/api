// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
)

// TestCopyPreservesEveryScalarField guards a whole class of bug rather than one field.
// The Copy methods are hand-written field-by-field constructors, so adding a field to a
// descriptor and forgetting to copy it compiles, passes every existing test, and then
// loses the value at the one moment it matters. RegistrarDescriptor.Default was added
// for multi-registrar routing and omitted from Copy: a child stack reaches its
// registrars through parent-descriptor inheritance, which copies, so the fleet's
// catch-all registrar arrived unmarked and every `.com` deploy failed with "no registrar
// of stack ... handles domain storage.simple-forge.com".
//
// Rather than assert on Default alone, walk the type: set every scalar field to a
// non-zero value via reflection and require the copy to carry it. A newly added scalar
// starts at its zero value here and the test fails until Copy is updated.
func TestCopyPreservesEveryScalarField(t *testing.T) {
	RegisterTestingT(t)

	original := RegistrarDescriptor{}
	v := reflect.ValueOf(&original).Elem()
	typ := v.Type()

	var covered []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Anonymous {
			// Config and Inherit are embedded and have their own Copy coverage.
			continue
		}
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.String:
			fv.SetString("non-zero-" + field.Name)
		case reflect.Bool:
			fv.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fv.SetInt(7)
		default:
			// A non-scalar field needs its own dedicated assertion; skip it here so
			// this test keeps reporting on the fields it can actually synthesise.
			continue
		}
		covered = append(covered, field.Name)
	}

	// If this trips, RegistrarDescriptor lost its scalar fields and the test below
	// would pass vacuously.
	Expect(covered).To(ContainElements("Type", "Default"))

	copied := original.Copy()
	cv := reflect.ValueOf(copied)
	for _, name := range covered {
		Expect(cv.FieldByName(name).Interface()).To(
			Equal(v.FieldByName(name).Interface()),
			"RegistrarDescriptor.Copy() dropped field %q — add it to the constructor in copy.go", name,
		)
	}
}

// TestCopyKeepsDefaultThroughAllRegistrars is the end-to-end shape of the same bug: the
// routing code reads Default off the descriptor it is handed, and what it is handed on a
// child deploy has been through PerStackResourcesDescriptor.Copy.
func TestCopyKeepsDefaultThroughAllRegistrars(t *testing.T) {
	RegisterTestingT(t)

	resources := PerStackResourcesDescriptor{
		Registrars: map[string]RegistrarDescriptor{
			"cloudflare": {Type: "cloudflare", Default: true},
			"yandex":     {Type: "yc-dns"},
		},
	}

	copied := resources.Copy()
	registrars, err := copied.AllRegistrars()
	Expect(err).To(BeNil())
	Expect(registrars).To(HaveLen(2))
	Expect(registrars["cloudflare"].Default).To(BeTrue(), "the catch-all must survive a copy")
	Expect(registrars["yandex"].Default).To(BeFalse())
}
