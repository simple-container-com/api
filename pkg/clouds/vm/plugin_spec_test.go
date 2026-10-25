// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"reflect"
	"testing"

	. "github.com/onsi/gomega"

	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// The drift guard described in plugin_spec.go's header.
//
// It is reflective rather than a hand-written fixture for one reason: a fixture only
// covers the fields whoever wrote it knew about, so adding an eleventh capability
// and forgetting the conversion would leave this test green. Filling by reflection
// means the new field is populated whether or not anyone remembered it, and the
// round-trip then drops it and fails.
func TestVmDriverSpecRoundTripsEveryField(t *testing.T) {
	RegisterTestingT(t)

	info := DriverInfo{
		Implemented: true,
		Unverified:  "the cloud exists but nobody has pointed this driver at it",
		RoadmapItem: "sc-ru-clouds-beyond-yandex",
	}
	fillEveryField(t, reflect.ValueOf(&info.Capabilities).Elem())

	Expect(DriverInfoFromProto(info.ToProto("some-cloud-vm"))).To(Equal(info))
}

// ToProto carries the driver name (which is not part of the row) and deliberately
// drops Plugin (which only core can know).
func TestVmDriverSpecCarriesTheDriverNameAndNotThePlugin(t *testing.T) {
	RegisterTestingT(t)

	spec := DriverInfo{Implemented: true, Plugin: "sc-plugin-someone-else"}.ToProto("some-cloud-vm")

	Expect(spec.GetDriver()).To(Equal("some-cloud-vm"))
	Expect(DriverInfoFromProto(spec).Plugin).To(BeEmpty())
}

// A plugin may legitimately send no capability block at all — it has measured
// nothing yet, or it was built against a bus revision that predates a field. That
// must read as the zero row ("not measured"), not panic on a nil message.
func TestDriverInfoFromProtoToleratesAbsentCapabilities(t *testing.T) {
	RegisterTestingT(t)

	info := DriverInfoFromProto(&scpluginv1.VmDriverSpec{
		Driver:      "some-cloud-vm",
		Implemented: true,
		RoadmapItem: "sc-ru-clouds-beyond-yandex",
	})

	Expect(info.Implemented).To(BeTrue())
	Expect(info.RoadmapItem).To(Equal("sc-ru-clouds-beyond-yandex"))
	Expect(info.Capabilities).To(Equal(Capabilities{}))
}

// fillEveryField sets every field of a struct to a non-zero value, and fails the
// test on a field kind it does not know how to fill — which is the point. A new
// Capabilities field of an unhandled kind stops here with a message naming it,
// rather than silently defaulting to its zero value and passing the round-trip.
func fillEveryField(t *testing.T, v reflect.Value) {
	t.Helper()
	for i := range v.NumField() {
		f := v.Field(i)
		name := v.Type().Field(i).Name
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int32, reflect.Int64:
			// Distinct per field, so a conversion that crosses two int fields over
			// fails rather than coincidentally matching.
			f.SetInt(int64(i + 1))
		case reflect.String:
			f.SetString(name)
		default:
			t.Fatalf("field %s.%s is a %s, which fillEveryField cannot populate: "+
				"extend this helper AND both directions of the conversion in plugin_spec.go",
				v.Type().Name(), name, f.Kind())
		}
	}
}
