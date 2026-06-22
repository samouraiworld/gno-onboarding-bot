package valoper

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client queries a gno RPC node's ABCI vm/qrender method over JSON-RPC.
type Client struct {
	endpoint string
	http     *http.Client
}

// NewClient returns a Client posting to the given JSON-RPC endpoint.
func NewClient(endpoint string) *Client {
	return &Client{endpoint: endpoint, http: &http.Client{Timeout: 10 * time.Second}}
}

type rpcRequest struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      int       `json:"id"`
	Method  string    `json:"method"`
	Params  rpcParams `json:"params"`
}

type rpcParams struct {
	Path   string `json:"path"`
	Data   string `json:"data"`
	Height string `json:"height"`
	Prove  bool   `json:"prove"`
}

type rpcResponse struct {
	Result struct {
		Response struct {
			ResponseBase struct {
				Error json.RawMessage `json:"Error"`
				Data  string          `json:"Data"`
				Log   string          `json:"Log"`
			} `json:"ResponseBase"`
		} `json:"response"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Render fetches the raw realm render string for realmPath.
func (c *Client) Render(ctx context.Context, realmPath string) (string, error) {
	reqBody, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "abci_query",
		Params: rpcParams{
			Path:   "vm/qrender",
			Data:   base64.StdEncoding.EncodeToString([]byte(realmPath)),
			Height: "0", Prove: false,
		},
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("qrender request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qrender status %d", resp.StatusCode)
	}

	var rr rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return "", fmt.Errorf("decode qrender response: %w", err)
	}
	if rr.Error != nil {
		return "", fmt.Errorf("rpc error: %s", rr.Error.Message)
	}
	if len(rr.Result.Response.ResponseBase.Error) > 0 && string(rr.Result.Response.ResponseBase.Error) != "null" {
		return "", fmt.Errorf("abci error: %s", rr.Result.Response.ResponseBase.Log)
	}

	data, err := base64.StdEncoding.DecodeString(rr.Result.Response.ResponseBase.Data)
	if err != nil {
		return "", fmt.Errorf("decode qrender data: %w", err)
	}
	return string(data), nil
}

type rpcValidatorsRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type validatorsResponse struct {
	Result *struct {
		Validators []struct {
			Address string `json:"address"`
		} `json:"validators"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ValidatorSet returns the set of active validator signing addresses reported by
// the node's `validators` RPC method, for O(1) membership checks. The gno node
// returns the full set in one call (no pagination).
func (c *Client) ValidatorSet(ctx context.Context) (map[string]struct{}, error) {
	reqBody, err := json.Marshal(rpcValidatorsRequest{
		JSONRPC: "2.0", ID: 1, Method: "validators", Params: struct{}{},
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("validators request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validators status %d", resp.StatusCode)
	}

	var vr validatorsResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode validators response: %w", err)
	}
	if vr.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", vr.Error.Message)
	}
	if vr.Result == nil {
		return nil, fmt.Errorf("validators response missing result")
	}
	set := make(map[string]struct{}, len(vr.Result.Validators))
	for _, v := range vr.Result.Validators {
		if v.Address != "" {
			set[v.Address] = struct{}{}
		}
	}
	return set, nil
}
