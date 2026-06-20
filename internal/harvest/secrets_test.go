package harvest

import (
	"reflect"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	mnemonic := "abandon ability able about above absent absorb abstract absurd abuse access accident"

	cases := []struct {
		name      string
		in        string
		wantKinds []string
		// substrings that must NOT survive in the cleaned output
		gone []string
		// substrings that MUST survive (legitimate content)
		kept []string
	}{
		{
			name:      "plain text",
			in:        "Node is synced at height 12345, here is my valoper link.",
			wantKinds: nil,
			kept:      []string{"synced at height 12345"},
		},
		{
			name:      "bare mention is not a leak",
			in:        "I stored my seed phrase and private key safely offline.",
			wantKinds: nil,
			kept:      []string{"seed phrase", "private key"},
		},
		{
			name:      "public tx hash survives",
			in:        "tx: A1B2C3D4E5F6071829304152637485960718293041526374859607182930AABB done",
			wantKinds: nil,
			kept:      []string{"A1B2C3D4E5F6071829304152637485960718293041526374859607182930AABB"},
		},
		{
			name:      "valoper address survives",
			in:        "my address is g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5",
			wantKinds: nil,
			kept:      []string{"g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5"},
		},
		{
			name:      "seed phrase redacted",
			in:        "here it is: " + mnemonic,
			wantKinds: []string{"seed_phrase"},
			gone:      []string{"abandon ability able", "accident"},
			kept:      []string{"[REDACTED:seed_phrase]", "here it is:"},
		},
		{
			name:      "short word run is not a mnemonic",
			in:        "the node is up and in sync now all good thanks team",
			wantKinds: nil,
			kept:      []string{"the node is up and in sync now all good thanks team"},
		},
		{
			name:      "private ip redacted",
			in:        "connect to 192.168.1.42 on the lan",
			wantKinds: []string{"private_ip"},
			gone:      []string{"192.168.1.42"},
			kept:      []string{"[REDACTED:private_ip]"},
		},
		{
			name:      "pem private key redacted",
			in:        "key:\n-----BEGIN PRIVATE KEY-----\nMIIabc123\n-----END PRIVATE KEY-----\nthanks",
			wantKinds: []string{"private_key"},
			gone:      []string{"MIIabc123"},
			kept:      []string{"[REDACTED:private_key]", "thanks"},
		},
		{
			name:      "multiple kinds sorted",
			in:        "ip 10.0.0.5 and phrase " + mnemonic,
			wantKinds: []string{"private_ip", "seed_phrase"},
			gone:      []string{"10.0.0.5", "abandon ability"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clean, kinds := Redact(c.in)
			if !reflect.DeepEqual(kinds, c.wantKinds) {
				t.Errorf("kinds = %v, want %v", kinds, c.wantKinds)
			}
			for _, g := range c.gone {
				if strings.Contains(clean, g) {
					t.Errorf("cleaned text still contains %q: %q", g, clean)
				}
			}
			for _, k := range c.kept {
				if !strings.Contains(clean, k) {
					t.Errorf("cleaned text dropped %q: %q", k, clean)
				}
			}
		})
	}
}
