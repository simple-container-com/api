package scan

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveVersion(t *testing.T) {
	RegisterTestingT(t)

	t.Run("explicit value wins over env", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TEST_VERSION_VAR", "9.9.9")
		Expect(resolveVersion("1.2.3", "SC_TEST_VERSION_VAR")).To(Equal("1.2.3"))
	})

	t.Run("falls back to env when explicit empty", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TEST_VERSION_VAR", "4.5.6")
		Expect(resolveVersion("", "SC_TEST_VERSION_VAR")).To(Equal("4.5.6"))
	})

	t.Run("empty when neither set", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TEST_VERSION_VAR", "")
		Expect(resolveVersion("", "SC_TEST_VERSION_VAR")).To(Equal(""))
	})
}

func TestNewScannerWithVersion_Grype(t *testing.T) {
	RegisterTestingT(t)

	t.Run("default version when nothing supplied", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_GRYPE_VERSION", "")
		s, err := NewScannerWithVersion(ScanToolGrype, "")
		Expect(err).ToNot(HaveOccurred())
		gs, ok := s.(*GrypeScanner)
		Expect(ok).To(BeTrue())
		Expect(gs.installVersion).To(Equal(DefaultGrypeVersion))
		Expect(gs.minVersion).To(Equal(DefaultGrypeVersion))
	})

	t.Run("explicit version overrides default", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_GRYPE_VERSION", "")
		s, err := NewScannerWithVersion(ScanToolGrype, "0.123.4")
		Expect(err).ToNot(HaveOccurred())
		gs := s.(*GrypeScanner)
		Expect(gs.installVersion).To(Equal("0.123.4"))
		Expect(gs.minVersion).To(Equal("0.123.4"))
	})

	t.Run("env var overrides default when no explicit", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_GRYPE_VERSION", "0.200.0")
		s, err := NewScannerWithVersion(ScanToolGrype, "")
		Expect(err).ToNot(HaveOccurred())
		gs := s.(*GrypeScanner)
		Expect(gs.installVersion).To(Equal("0.200.0"))
		Expect(gs.minVersion).To(Equal("0.200.0"))
	})
}

func TestNewScannerWithVersion_Trivy(t *testing.T) {
	RegisterTestingT(t)

	t.Run("default version when nothing supplied", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TRIVY_VERSION", "")
		s, err := NewScannerWithVersion(ScanToolTrivy, "")
		Expect(err).ToNot(HaveOccurred())
		ts, ok := s.(*TrivyScanner)
		Expect(ok).To(BeTrue())
		Expect(ts.installVersion).To(Equal(DefaultTrivyVersion))
		Expect(ts.minVersion).To(Equal(DefaultTrivyVersion))
	})

	t.Run("explicit version overrides default", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TRIVY_VERSION", "")
		s, err := NewScannerWithVersion(ScanToolTrivy, "0.88.0")
		Expect(err).ToNot(HaveOccurred())
		ts := s.(*TrivyScanner)
		Expect(ts.installVersion).To(Equal("0.88.0"))
		Expect(ts.minVersion).To(Equal("0.88.0"))
	})

	t.Run("env var overrides default when no explicit", func(t *testing.T) {
		RegisterTestingT(t)
		t.Setenv("SC_TRIVY_VERSION", "0.99.9")
		s, err := NewScannerWithVersion(ScanToolTrivy, "")
		Expect(err).ToNot(HaveOccurred())
		ts := s.(*TrivyScanner)
		Expect(ts.installVersion).To(Equal("0.99.9"))
		Expect(ts.minVersion).To(Equal("0.99.9"))
	})
}

func TestNewScannerWithVersion_Unsupported(t *testing.T) {
	RegisterTestingT(t)

	s, err := NewScannerWithVersion("nessus", "1.0.0")
	Expect(err).To(HaveOccurred())
	Expect(s).To(BeNil())
	Expect(err.Error()).To(ContainSubstring("unsupported scan tool"))
}

func TestNewScanner_AllToolUnsupported(t *testing.T) {
	RegisterTestingT(t)
	// ScanToolAll is a valid config value for merging but is NOT a concrete scanner.
	s, err := NewScanner(ScanToolAll)
	Expect(err).To(HaveOccurred())
	Expect(s).To(BeNil())
}
