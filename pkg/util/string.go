package util

import (
	"regexp"
	"strings"
)

func TrimStringMiddle(str string, maxLen int, sep string) string {
	if len(str) > maxLen {
		return str[:maxLen/2] + sep + str[len(str)-maxLen/2:]
	}
	return str
}

var (
	matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

func ToEnvVariableName(str string) string {
	return strings.ReplaceAll(strings.ToUpper(ToSnakeCase(str)), "-", "_")
}
