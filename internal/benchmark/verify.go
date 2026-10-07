package benchmark

import (
	"strconv"
	"strings"
)

func (v Verify) HasChecks() bool {
	return len(v.CoreTests) > 0 || len(v.RegressionTests) > 0 || v.Command != "" || v.OutputContains != "" || v.OutputEquals != nil || v.IntegerSequence != nil
}

// CheckOutput checks all configured assertions. Exact equality preserves bytes.
func (v Verify) CheckOutput(text string) string {
	if v.OutputContains != "" && !strings.Contains(text, v.OutputContains) {
		return "Output substring assertion failed"
	}
	if v.OutputEquals != nil && text != *v.OutputEquals {
		return "Exact output assertion failed"
	}
	if seq := v.IntegerSequence; seq != nil {
		// Permit CRLF and one final line terminator, but no missing, duplicate,
		// reordered, extra or blank lines.
		lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
		if len(lines) != seq.End-seq.Start+2 {
			return "Integer sequence line count mismatch"
		}
		for i := seq.Start; i <= seq.End; i++ {
			if lines[i-seq.Start] != strconv.Itoa(i) {
				return "Integer sequence content mismatch"
			}
		}
		if lines[len(lines)-1] != seq.EndMarker {
			return "Integer sequence end marker mismatch"
		}
	}
	return ""
}
