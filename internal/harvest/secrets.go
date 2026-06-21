package harvest

import (
	"regexp"
	"sort"
	"strings"
)

// Secret detection is deliberately conservative: it must not copy a leaked
// secret into the Evidence tab or harvest.json, but it also must not cry wolf on
// every message that merely says "seed phrase". Each pattern matches
// secret-shaped content, never a bare mention of the word.

// A bare 64-hex string is intentionally NOT matched: a public transaction hash
// (which candidates are asked to post as evidence) and a 32-byte private key are
// shape-identical, so redacting it would hide legitimate evidence and raise a
// false leak flag. Described key mishandling is left to the skill's judgment.
var (
	rePEMPrivateKey = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	rePrivateIP     = regexp.MustCompile(`\b(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})\b`)
	reMnemonicWord  = regexp.MustCompile(`^[a-z]{3,8}$`)
)

// redaction is one secret pattern and the label that replaces it.
type redaction struct {
	kind string
	re   *regexp.Regexp
}

var redactions = []redaction{
	{"private_key", rePEMPrivateKey},
	{"private_ip", rePrivateIP},
}

// Redact replaces any detected secret in text with `[REDACTED:<kind>]` and
// returns the cleaned text plus the sorted, de-duplicated kinds found. A nil
// kinds slice means nothing was detected.
func Redact(text string) (clean string, kinds []string) {
	found := map[string]bool{}
	clean = text

	for _, r := range redactions {
		if r.re.MatchString(clean) {
			found[r.kind] = true
			clean = r.re.ReplaceAllString(clean, "[REDACTED:"+r.kind+"]")
		}
	}

	if cleaned, ok := redactMnemonic(clean); ok {
		found["seed_phrase"] = true
		clean = cleaned
	}

	for k := range found {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return clean, kinds
}

// redactMnemonic looks for a run of 12 or more space-separated lowercase words
// (3-8 letters each), the shape of a BIP39 seed phrase, and blanks the run. It
// is intentionally strict on the run length so ordinary prose does not trip it.
func redactMnemonic(text string) (string, bool) {
	fields := strings.Fields(text)
	var out []string
	runLen, found := 0, false
	flush := func(end int) {
		if runLen >= 12 {
			found = true
			out = append(out, "[REDACTED:seed_phrase]")
		} else {
			out = append(out, fields[end-runLen:end]...)
		}
		runLen = 0
	}

	for i, f := range fields {
		if reMnemonicWord.MatchString(f) {
			runLen++
			continue
		}
		flush(i)
		out = append(out, f)
	}
	flush(len(fields))

	if !found {
		return text, false
	}
	return strings.Join(out, " "), true
}
