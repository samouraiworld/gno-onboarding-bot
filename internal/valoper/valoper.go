// Package valoper reads validator-operator profiles from the on-chain
// r/gnops/valopers realm so the bot can auto-fill a candidate submission from a
// single operator address.
package valoper

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// RealmPath is the qrender realm path (without the ":addr" suffix).
const RealmPath = "gno.land/r/gnops/valopers"

var (
	ErrInvalidInput  = errors.New("input is not a valopers link or g1 address")
	ErrNotRegistered = errors.New("no valoper registered for that address")
	ErrUnparseable   = errors.New("realm render could not be parsed")
)

var addrRe = regexp.MustCompile(`^g1[0-9a-z]{38}$`)

// Renderer fetches the raw realm render string for a realm path.
type Renderer interface {
	Render(ctx context.Context, realmPath string) (string, error)
}

// AddressFromInput extracts a gno address from a pasted valopers profile link or
// a bare g1 address.
func AddressFromInput(s string) (string, error) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "valopers:"); i >= 0 {
		s = s[i+len("valopers:"):]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if !addrRe.MatchString(s) {
		return "", ErrInvalidInput
	}
	return s, nil
}

// ParseRender extracts the moniker, operator address, signing address, and description from a
// single-valoper realm render.
func ParseRender(raw string) (moniker, operatorAddr, signingAddr, description string, err error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if t := strings.TrimSpace(raw); strings.HasPrefix(t, "unknown address") || strings.HasPrefix(t, "invalid address") {
		return "", "", "", "", ErrNotRegistered
	}

	const opMarker = "- Operator Address:"
	const signMarker = "- Signing Address:"
	lines := strings.Split(raw, "\n")
	monikerIdx, opIdx := -1, -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if monikerIdx == -1 && strings.HasPrefix(t, "## ") {
			monikerIdx = i
			moniker = strings.TrimSpace(strings.TrimPrefix(t, "## "))
			continue
		}
		if opIdx == -1 && strings.HasPrefix(t, opMarker) {
			opIdx = i
			operatorAddr = strings.TrimSpace(strings.TrimPrefix(t, opMarker))
			continue
		}
		if signingAddr == "" && strings.HasPrefix(t, signMarker) {
			signingAddr = strings.TrimSpace(strings.TrimPrefix(t, signMarker))
		}
	}
	if monikerIdx == -1 || opIdx == -1 || moniker == "" || operatorAddr == "" {
		return "", "", "", "", ErrUnparseable
	}
	description = strings.TrimSpace(strings.Join(lines[monikerIdx+1:opIdx], "\n"))
	return moniker, operatorAddr, signingAddr, description, nil
}

// ProfileURL builds the gnoweb profile URL for a valoper address.
func ProfileURL(gnowebBase, addr string) string {
	return strings.TrimRight(gnowebBase, "/") + "/r/gnops/valopers:" + addr
}
