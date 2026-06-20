package valoper

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRender_Success(t *testing.T) {
	want := "Valoper's details:\n## SamouraiCoop\nintro\n\n- Operator Address: g1abc\n"
	body := `{"jsonrpc":"2.0","id":1,"result":{"response":{"ResponseBase":{"Error":null,"Data":"` +
		base64.StdEncoding.EncodeToString([]byte(want)) + `","Events":null,"Log":"","Info":""}}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req struct {
			Method string `json:"method"`
			Params struct {
				Path string `json:"path"`
				Data string `json:"data"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "abci_query" || req.Params.Path != "vm/qrender" {
			t.Errorf("method=%q path=%q", req.Method, req.Params.Path)
		}
		dec, _ := base64.StdEncoding.DecodeString(req.Params.Data)
		if string(dec) != "gno.land/r/gnops/valopers:g1abc" {
			t.Errorf("data = %q", dec)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).Render(context.Background(), "gno.land/r/gnops/valopers:g1abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestClientRender_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).Render(context.Background(), "x"); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}

func TestClientRender_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"message":"boom"}}`))
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).Render(context.Background(), "x"); err == nil {
		t.Fatal("expected error on rpc-level error")
	}
}
