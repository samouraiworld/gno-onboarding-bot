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
