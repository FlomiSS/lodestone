package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// NodeError ist eine Fehlerantwort eines Nodes.
type NodeError struct {
	Status  int
	Message string
}

func (e *NodeError) Error() string { return e.Message }

// ErrNodeOffline meldet, dass der Node nicht antwortet.
var ErrNodeOffline = errors.New("der Node ist nicht erreichbar")

// NodeClient spricht mit dem Daemon eines Nodes.
type NodeClient struct {
	node Node
	http *http.Client
}

func newNodeClient(node Node) *NodeClient {
	return &NodeClient{node: node, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *NodeClient) url(path string) string {
	return strings.TrimRight(c.node.URL, "/") + path
}

// Do sendet eine Anfrage mit JSON-Körper und liest die JSON-Antwort in out (kann nil sein).
func (c *NodeClient) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.url(path), reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.node.Token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w (%s): %v", ErrNodeOffline, c.node.Name, err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &failure)
		if failure.Error == "" {
			failure.Error = fmt.Sprintf("Node antwortet mit %d", response.StatusCode)
		}
		return &NodeError{Status: response.StatusCode, Message: failure.Error}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("unlesbare Antwort des Nodes: %w", err)
		}
	}
	return nil
}

// Raw liefert den Text einer Antwort (für Dateiinhalte).
func (c *NodeClient) Raw(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.url(path), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.node.Token)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w (%s): %v", ErrNodeOffline, c.node.Name, err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if response.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &failure)
		if failure.Error == "" {
			failure.Error = fmt.Sprintf("Node antwortet mit %d", response.StatusCode)
		}
		return nil, &NodeError{Status: response.StatusCode, Message: failure.Error}
	}
	return data, nil
}

// Proxy reicht eine Browser-Anfrage an einen Pfad des Nodes weiter. Der Pfad ersetzt den der Anfrage; Abfrageparameter
// bleiben. Uploads und Downloads laufen als Strom durch.
func (c *NodeClient) Proxy(w http.ResponseWriter, r *http.Request, path string) {
	target, err := url.Parse(c.node.URL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ungültige Adresse des Nodes"})
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(out *httputil.ProxyRequest) {
			out.SetURL(target)
			out.Out.URL.Path = strings.TrimRight(target.Path, "/") + path
			out.Out.URL.RawPath = ""
			out.Out.URL.RawQuery = r.URL.RawQuery
			out.Out.Host = target.Host
			for _, name := range []string{"Cookie", "Authorization", csrfHeader, "Origin", "Referer"} {
				out.Out.Header.Del(name)
			}
			out.Out.Header.Set("Authorization", "Bearer "+c.node.Token)
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "der Node " + c.node.Name + " ist nicht erreichbar"})
		},
	}
	proxy.ServeHTTP(w, r)
}

// DialWS verbindet sich mit dem Websocket eines Servers auf dem Node.
func (c *NodeClient) DialWS(ctx context.Context, path string) (*websocket.Conn, error) {
	address := strings.Replace(strings.Replace(c.url(path), "https://", "wss://", 1), "http://", "ws://", 1)
	conn, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.node.Token}},
	})
	if err != nil {
		return nil, fmt.Errorf("%w (%s): %v", ErrNodeOffline, c.node.Name, err)
	}
	conn.SetReadLimit(4 << 20)
	return conn, nil
}
