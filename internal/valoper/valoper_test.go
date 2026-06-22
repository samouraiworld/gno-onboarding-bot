package valoper

import (
	"errors"
	"testing"
)

const addr = "g1n9y62agq998jt8w59az60xcqlftjknjg2grhn4"

func TestAddressFromInput(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"full url", "https://gnoweb.test-13.gnoland.network/r/gnops/valopers:" + addr, addr, nil},
		{"url trailing slash", "https://x/r/gnops/valopers:" + addr + "/", addr, nil},
		{"url with query", "https://x/r/gnops/valopers:" + addr + "?a=1", addr, nil},
		{"bare address", addr, addr, nil},
		{"surrounding spaces", "  " + addr + "  ", addr, nil},
		{"empty", "", "", ErrInvalidInput},
		{"not an address", "https://example.com/hello", "", ErrInvalidInput},
		{"wrong prefix", "cosmos1abcdefghijklmnopqrstuvwxyz0123456789", "", ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddressFromInput(tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

const fullRender = `Valoper's details:
## SamouraiCoop
Multi-line intro.

Second paragraph with a [link](https://x).

- Operator Address: g1n9y62agq998jt8w59az60xcqlftjknjg2grhn4
- Signing Address: g1k7asng8uzf74xs0tsrfwytldl76hs4l3asglym
- Signing PubKey: gpub1xyz
- Server Type: cloud

[Profile link](/r/demo/profile:u/g1n9y62agq998jt8w59az60xcqlftjknjg2grhn4)
`

func TestParseRender(t *testing.T) {
	moniker, gotAddr, signing, desc, err := ParseRender(fullRender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moniker != "SamouraiCoop" {
		t.Errorf("moniker = %q", moniker)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q", gotAddr)
	}
	if signing != "g1k7asng8uzf74xs0tsrfwytldl76hs4l3asglym" {
		t.Errorf("signing = %q", signing)
	}
	want := "Multi-line intro.\n\nSecond paragraph with a [link](https://x)."
	if desc != want {
		t.Errorf("desc = %q, want %q", desc, want)
	}
}

func TestParseRender_EmptyDescription(t *testing.T) {
	raw := "Valoper's details:\n## Solo\n- Operator Address: g1abc\n- Server Type: cloud\n"
	moniker, gotAddr, signing, desc, err := ParseRender(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moniker != "Solo" || gotAddr != "g1abc" || signing != "" || desc != "" {
		t.Errorf("got moniker=%q addr=%q signing=%q desc=%q", moniker, gotAddr, signing, desc)
	}
}

func TestParseRender_Unknown(t *testing.T) {
	for _, raw := range []string{"unknown address " + addr, "invalid address foo"} {
		if _, _, _, _, err := ParseRender(raw); !errors.Is(err, ErrNotRegistered) {
			t.Errorf("ParseRender(%q) err = %v, want ErrNotRegistered", raw, err)
		}
	}
}

func TestParseRender_MissingMarkers(t *testing.T) {
	if _, _, _, _, err := ParseRender("garbage with no markers"); !errors.Is(err, ErrUnparseable) {
		t.Errorf("err = %v, want ErrUnparseable", err)
	}
}

func TestProfileURL(t *testing.T) {
	want := "https://gnoweb.test-13.gnoland.network/r/gnops/valopers:" + addr
	if got := ProfileURL("https://gnoweb.test-13.gnoland.network", addr); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := ProfileURL("https://gnoweb.test-13.gnoland.network/", addr); got != want {
		t.Errorf("trailing slash: got %q, want %q", got, want)
	}
}
