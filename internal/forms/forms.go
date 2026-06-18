package forms

import "strings"

func SplitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

type Field struct {
	Label string
	Value string
}

func MissingRequired(fields []Field) []string {
	var missing []string
	for _, f := range fields {
		if strings.TrimSpace(f.Value) == "" {
			missing = append(missing, f.Label)
		}
	}
	return missing
}
