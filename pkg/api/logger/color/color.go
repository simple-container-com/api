package color

import "github.com/fatih/color"

func GreenFmt(fmt string, args ...any) string {
	return color.GreenString(fmt, args...)
}

func BlueBgFmt(str string, args ...any) string {
	return color.New(color.BgBlue).Sprintf(str, args...)
}

func Green(str any) string {
	return color.GreenString("%s", str)
}

func MagentaFmt(fmt string, args ...any) string {
	return color.MagentaString(fmt, args...)
}

func YellowFmt(fmt string, args ...any) string {
	return color.YellowString(fmt, args...)
}

func Yellow(str any) string {
	return color.YellowString("%s", str)
}

func RedFmt(fmt string, args ...any) string {
	return color.RedString(fmt, args...)
}

func Red(str any) string {
	return color.RedString("%s", str)
}

func BlueFmt(fmt string, args ...any) string {
	return color.BlueString(fmt, args...)
}

func Blue(str any) string {
	return color.BlueString("%s", str)
}

func CyanFmt(fmt string, args ...any) string {
	return color.CyanString(fmt, args...)
}

func Cyan(str any) string {
	return color.CyanString("%s", str)
}

func GrayFmt(fmt string, args ...any) string {
	return color.WhiteString(fmt, args...) // Use white for gray
}

func Gray(str any) string {
	return color.WhiteString("%s", str)
}

func BlueBold(str string) string {
	return color.New(color.FgBlue, color.Bold).Sprint(str)
}

func GrayString(str string) string {
	return color.WhiteString(str)
}

func YellowString(str string) string {
	return color.YellowString(str)
}

func CyanString(str string) string {
	return color.CyanString(str)
}

func GreenString(str string) string {
	return color.GreenString(str)
}

func RedString(str string) string {
	return color.RedString(str)
}

func BlueString(str string) string {
	return color.BlueString(str)
}

func WhiteString(str string) string {
	return color.WhiteString(str)
}

func BoldFmt(format string, args ...any) string {
	return color.New(color.Bold).Sprintf(format, args...)
}

// Assistant chat colors for better readability
func AssistantText(str string) string {
	return color.New(color.FgHiCyan).Sprint(str)
}

func AssistantCode(str string) string {
	return color.New(color.FgHiYellow).Sprint(str)
}

func AssistantHeader(str string) string {
	return color.New(color.FgHiMagenta, color.Bold).Sprint(str)
}

func AssistantEmphasis(str string) string {
	return color.New(color.FgHiGreen).Sprint(str)
}

func YellowBold(str string) string {
	return color.New(color.FgYellow, color.Bold).Sprint(str)
}
