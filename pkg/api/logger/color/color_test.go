package color

import (
	"testing"

	. "github.com/onsi/gomega"

	fatihcolor "github.com/fatih/color"
)

// The color helpers wrap fatih/color. fatih/color short-circuits to a
// bare string when it detects a non-TTY (e.g., this test process), so
// asserting on the exact escape sequence is brittle across CI / local
// runs. Instead we force NoColor=false and assert that the helpers:
//   1) return a non-empty string when given a non-empty input
//   2) include the input substring (escape codes only wrap it)
// This covers every helper without depending on terminal capability
// detection.

func withColorEnabled(t *testing.T, fn func()) {
	t.Helper()
	prev := fatihcolor.NoColor
	fatihcolor.NoColor = false
	t.Cleanup(func() { fatihcolor.NoColor = prev })
	fn()
}

// TestFmtHelpers exercises every "Fmt"-suffixed helper that takes a
// printf-style format + args.
func TestFmtHelpers(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string, ...any) string
	}{
		{"GreenFmt", GreenFmt},
		{"BlueBgFmt", BlueBgFmt},
		{"MagentaFmt", MagentaFmt},
		{"YellowFmt", YellowFmt},
		{"RedFmt", RedFmt},
		{"BlueFmt", BlueFmt},
		{"CyanFmt", CyanFmt},
		{"BoldFmt", BoldFmt},
		{"GrayFmt", GrayFmt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			withColorEnabled(t, func() {
				got := tc.fn("hello %s", "world")
				Expect(got).ToNot(BeEmpty())
				Expect(got).To(ContainSubstring("hello world"))
			})
		})
	}
}

// TestAnyHelpers exercises every helper that takes a single value
// to colour (Green / Red / Blue / Yellow / Cyan / etc.).
func TestAnyHelpers(t *testing.T) {
	cases := []struct {
		name string
		fn   func(any) string
	}{
		{"Green", Green},
		{"Yellow", Yellow},
		{"Red", Red},
		{"Blue", Blue},
		{"Cyan", Cyan},
		{"Gray", Gray},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			withColorEnabled(t, func() {
				got := tc.fn("token")
				Expect(got).ToNot(BeEmpty())
				Expect(got).To(ContainSubstring("token"))
			})
		})
	}
}

// TestStringHelpers covers the *String variants that accept a string
// directly (no fmt).
func TestStringHelpers(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) string
	}{
		{"GreenString", GreenString},
		{"RedString", RedString},
		{"BlueString", BlueString},
		{"WhiteString", WhiteString},
		{"GrayString", GrayString},
		{"YellowString", YellowString},
		{"CyanString", CyanString},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			withColorEnabled(t, func() {
				got := tc.fn("payload")
				Expect(got).ToNot(BeEmpty())
				Expect(got).To(ContainSubstring("payload"))
			})
		})
	}
}

// TestAssistantHelpers covers the chat-styled helpers used by the
// AI assistant subsystem.
func TestAssistantHelpers(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) string
	}{
		{"AssistantText", AssistantText},
		{"AssistantCode", AssistantCode},
		{"AssistantHeader", AssistantHeader},
		{"AssistantEmphasis", AssistantEmphasis},
		{"YellowBold", YellowBold},
		{"BlueBold", BlueBold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			withColorEnabled(t, func() {
				got := tc.fn("chat-line")
				Expect(got).ToNot(BeEmpty())
				Expect(got).To(ContainSubstring("chat-line"))
			})
		})
	}
}

func TestHelpersHandleEmptyInput(t *testing.T) {
	RegisterTestingT(t)
	withColorEnabled(t, func() {
		// All helpers should be safe with empty input — no panic, returns a string.
		Expect(GreenFmt("")).ToNot(BeNil())
		Expect(Green("")).ToNot(BeNil())
		Expect(GreenString("")).ToNot(BeNil())
		Expect(AssistantText("")).ToNot(BeNil())
	})
}
