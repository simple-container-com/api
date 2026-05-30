package util

import (
	"testing"

	. "github.com/onsi/gomega"
)

type sampleTarget struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Tags  []string `json:"tags"`
}

func TestToObjectViaJson_StructToStruct(t *testing.T) {
	RegisterTestingT(t)

	from := map[string]any{
		"name":  "alpha",
		"count": 7,
		"tags":  []string{"a", "b"},
	}
	to := &sampleTarget{}

	out, err := ToObjectViaJson(from, to)
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeNil())
	Expect(out.Name).To(Equal("alpha"))
	Expect(out.Count).To(Equal(7))
	Expect(out.Tags).To(Equal([]string{"a", "b"}))
}

func TestToObjectViaJson_MarshalError(t *testing.T) {
	RegisterTestingT(t)

	// channel types are not JSON-marshalable; this exercises the
	// json.Marshal error branch.
	from := make(chan int)
	to := &sampleTarget{}
	_, err := ToObjectViaJson(from, to)
	Expect(err).To(HaveOccurred())
}

func TestToObjectViaJson_UnmarshalTypeMismatch(t *testing.T) {
	RegisterTestingT(t)

	// "count" is a string in the source but int in the target → unmarshal error.
	from := map[string]any{
		"name":  "alpha",
		"count": "not-a-number",
	}
	to := &sampleTarget{}

	_, err := ToObjectViaJson(from, to)
	Expect(err).To(HaveOccurred())
}

func TestToObjectViaJson_EmptySource(t *testing.T) {
	RegisterTestingT(t)

	from := map[string]any{}
	to := &sampleTarget{}

	out, err := ToObjectViaJson(from, to)
	Expect(err).ToNot(HaveOccurred())
	Expect(out.Name).To(Equal(""))
	Expect(out.Count).To(Equal(0))
}
