package utils

// TruncateString truncate the string after at most length characters
func TruncateString(str string, length int) string {
	if length <= 0 {
		return ""
	}

	runes := []rune(str)
	if len(runes) <= length {
		return str
	}

	return string(runes[:length])
}
