package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestExtractURLs(t *testing.T) {
	RegisterTestingT(t)

	t.Run("copies urls into a new slice", func(t *testing.T) {
		RegisterTestingT(t)
		v := grypeVulnerability{URLs: []string{"https://a", "https://b"}}
		got := extractURLs(v)
		Expect(got).To(Equal([]string{"https://a", "https://b"}))

		// Must be a copy, not the same backing array (mutation safety).
		got[0] = "mutated"
		Expect(v.URLs[0]).To(Equal("https://a"))
	})

	t.Run("nil urls yields nil", func(t *testing.T) {
		RegisterTestingT(t)
		Expect(extractURLs(grypeVulnerability{})).To(BeNil())
	})
}

func TestExtractCVSS(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		cvss []grypeCVSS
		want float64
	}{
		{
			name: "no entries returns zero",
			cvss: nil,
			want: 0,
		},
		{
			name: "single entry",
			cvss: []grypeCVSS{{Metrics: grypeCVSSMetrics{BaseScore: 7.5}}},
			want: 7.5,
		},
		{
			name: "picks the maximum base score",
			cvss: []grypeCVSS{
				{Metrics: grypeCVSSMetrics{BaseScore: 4.0}},
				{Metrics: grypeCVSSMetrics{BaseScore: 9.1}},
				{Metrics: grypeCVSSMetrics{BaseScore: 6.6}},
			},
			want: 9.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(extractCVSS(grypeVulnerability{Cvss: tt.cvss})).To(Equal(tt.want))
		})
	}
}
