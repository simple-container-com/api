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
