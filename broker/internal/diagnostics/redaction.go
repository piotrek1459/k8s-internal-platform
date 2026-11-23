package diagnostics

import (
	"sort"
	"strings"

	"k8s-internal-platform/broker/internal/platform"
)

// mask merges overlapping matches before replacing them. This avoids exposing
// a suffix of one secret when another overlapping match was replaced first.
// For a bounded read, a trailing prefix is covered as well, so truncation cannot
// turn a complete known secret into a visible partial value.
func (r redactor) mask(text string, truncated bool) (string, error) {
	type span struct{ start, end int }
	spans := []span{}
	for _, pattern := range r.patterns {
		for start := 0; start < len(text); {
			index := strings.Index(text[start:], pattern)
			if index < 0 {
				break
			}
			index += start
			spans = append(spans, span{index, index + len(pattern)})
			if len(spans) > 8192 {
				return "", platform.Errorf(platform.CodeDependencyUnavailable, "diagnostic redaction match limit reached")
			}
			start = index + 1
		}
		if truncated {
			if length := suffixPrefix(text, pattern); length > 0 {
				spans = append(spans, span{len(text) - length, len(text)})
			}
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end > spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	var result strings.Builder
	position := 0
	for i := 0; i < len(spans); {
		start, end := spans[i].start, spans[i].end
		i++
		for i < len(spans) && spans[i].start <= end {
			end = max(end, spans[i].end)
			i++
		}
		result.WriteString(text[position:start])
		result.WriteString("[REDACTED]")
		position = end
	}
	result.WriteString(text[position:])
	return result.String(), nil
}

// KMP computes the longest suffix matching a proper pattern prefix in linear
// time, even for repetitive values. Only the bounded tail can contain it.
func suffixPrefix(text, pattern string) int {
	if len(pattern) < 2 {
		return 0
	}
	prefix := make([]int, len(pattern))
	for i, j := 1, 0; i < len(pattern); i++ {
		for j > 0 && pattern[i] != pattern[j] {
			j = prefix[j-1]
		}
		if pattern[i] == pattern[j] {
			j++
		}
		prefix[i] = j
	}
	j := 0
	for i := max(0, len(text)-len(pattern)); i < len(text); i++ {
		for j > 0 && (j == len(pattern) || text[i] != pattern[j]) {
			j = prefix[j-1]
		}
		if text[i] == pattern[j] {
			j++
		}
	}
	if j == len(pattern) {
		return 0
	}
	return j
}
