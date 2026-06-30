// Package cueerr formats CUE evaluation errors into concise, deduplicated
// messages. CUE's default rendering of a failed disjunction (e.g. a bounded
// field with a default) is noisy: it repeats the offending value, wraps it in
// "N errors in empty disjunction", and surfaces the default branch as a
// misleading "conflicting values <default> and <value>". This package flattens
// the structured leaf errors, drops that noise, dedupes, and preserves the
// original error for [errors.Is] / [errors.As].
package cueerr

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/errors"
)

// Wrap wraps a CUE error with a formatted message and a concise summary of the
// underlying field errors. The wrapped error Unwraps to err.
func Wrap(err error, format string, args ...any) error {
	return &wrapped{msg: fmt.Sprintf(format, args...), err: err}
}

type wrapped struct {
	msg string
	err error
}

func (w *wrapped) Unwrap() error { return w.err }

func (w *wrapped) Error() string {
	if summary := summarize(w.err); summary != "" {
		return w.msg + ": " + summary
	}
	return w.msg + ": " + w.err.Error()
}

// summarize flattens CUE's leaf errors into one concise line: it drops the
// "N errors in empty disjunction" wrappers, prefers a specific bound/value error
// over the default-branch "conflicting values" noise for the same field, dedupes
// identical messages, and joins what remains with "; ".
func summarize(err error) string {
	leaves := errors.Errors(err)
	if len(leaves) == 0 {
		return ""
	}

	// Collect candidate messages per field path, preserving first-seen order.
	order := make([]string, 0, len(leaves))
	byPath := make(map[string][]string)
	for _, e := range leaves {
		msg := strings.TrimSpace(e.Error())
		if msg == "" || strings.Contains(msg, "errors in empty disjunction") {
			continue
		}
		key := strings.Join(e.Path(), ".")
		if _, seen := byPath[key]; !seen {
			order = append(order, key)
		}
		byPath[key] = append(byPath[key], msg)
	}

	var out []string
	seen := make(map[string]bool)
	for _, key := range order {
		for _, msg := range filterPathMessages(byPath[key]) {
			if !seen[msg] {
				seen[msg] = true
				out = append(out, msg)
			}
		}
	}
	return strings.Join(out, "; ")
}

// filterPathMessages drops the default-branch "conflicting values" noise for a
// field when a more specific bound/value error is present for the same field.
func filterPathMessages(msgs []string) []string {
	specific := false
	for _, m := range msgs {
		if strings.Contains(m, "out of bound") || strings.Contains(m, "invalid value") {
			specific = true
			break
		}
	}
	if !specific {
		return msgs
	}
	kept := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if strings.Contains(m, "conflicting values") {
			continue
		}
		kept = append(kept, m)
	}
	return kept
}
