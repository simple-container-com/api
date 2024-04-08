package util

func TrimStringMiddle(str string, maxLen int, sep string) string {
	if len(str) > maxLen {
		return str[:maxLen/2] + sep + str[len(str)-maxLen/2:]
	}
	return str
}
