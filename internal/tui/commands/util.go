package commands

// truncateStr truncates s to at most n runes, appending "…" if truncated.
// Using rune length avoids over-counting multi-byte UTF-8 characters.
func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
