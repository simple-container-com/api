package tools

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestParseVersion(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		input   string
		wantErr bool
		major   int
		minor   int
		patch   int
	}{
		{"1.2.3", false, 1, 2, 3},
		{"v1.2.3", false, 1, 2, 3},
		{"0.0.1", false, 0, 0, 1},
		{"10.20.30", false, 10, 20, 30},
		{"1.2", false, 1, 2, 0},
		{"v2.5", false, 2, 5, 0},
		{"1.2.3-beta", false, 1, 2, 3},
		{"invalid", true, 0, 0, 0},
		{"", true, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			RegisterTestingT(t)
			v, err := ParseVersion(tt.input)
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(v.Major).To(Equal(tt.major))
			Expect(v.Minor).To(Equal(tt.minor))
			Expect(v.Patch).To(Equal(tt.patch))
		})
	}
}

func TestVersionIsAtLeast(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		v1   string
		v2   string
		want bool
	}{
		{"same version", "1.2.3", "1.2.3", true},
		{"higher major", "2.0.0", "1.9.9", true},
		{"lower major", "1.0.0", "2.0.0", false},
		{"higher minor", "1.5.0", "1.4.9", true},
		{"lower minor", "1.3.0", "1.4.0", false},
		{"higher patch", "1.2.4", "1.2.3", true},
		{"lower patch", "1.2.2", "1.2.3", false},
		{"complex comparison 1", "3.0.2", "3.0.1", true},
		{"complex comparison 2", "1.41.0", "1.40.9", true},
		{"zero version", "0.0.0", "0.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			v1, err := ParseVersion(tt.v1)
			Expect(err).ToNot(HaveOccurred())
			v2, err := ParseVersion(tt.v2)
			Expect(err).ToNot(HaveOccurred())

			Expect(v1.IsAtLeast(v2)).To(Equal(tt.want))
		})
	}
}

func TestVersionCompare(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{"equal", "1.2.3", "1.2.3", 0},
		{"v1 greater major", "2.0.0", "1.9.9", 1},
		{"v1 less major", "1.0.0", "2.0.0", -1},
		{"v1 greater minor", "1.5.0", "1.4.0", 1},
		{"v1 less minor", "1.3.0", "1.4.0", -1},
		{"v1 greater patch", "1.2.4", "1.2.3", 1},
		{"v1 less patch", "1.2.2", "1.2.3", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			v1, _ := ParseVersion(tt.v1)
			v2, _ := ParseVersion(tt.v2)

			Expect(v1.Compare(v2)).To(Equal(tt.want))
		})
	}
}

func TestVersionString(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		major int
		minor int
		patch int
		want  string
	}{
		{1, 2, 3, "1.2.3"},
		{0, 0, 1, "0.0.1"},
		{10, 20, 30, "10.20.30"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			RegisterTestingT(t)
			v := &Version{
				Major: tt.major,
				Minor: tt.minor,
				Patch: tt.patch,
			}
			Expect(v.String()).To(Equal(tt.want))
		})
	}
}

func TestVersionCheckerExtractVersion(t *testing.T) {
	RegisterTestingT(t)

	checker := NewVersionChecker()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"simple version", "version 1.2.3", "1.2.3"},
		{"v prefix", "v1.2.3", "1.2.3"},
		{"tool name with version", "cosign version 3.0.2", "3.0.2"},
		{"multiline output", "Tool Info\nversion 2.5.1\nOther info", "2.5.1"},
		{"no version", "some output", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(checker.extractVersion(tt.output)).To(Equal(tt.want))
		})
	}
}
