package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Request holds a GraphQL request.
type Request struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// Response holds a GraphQL response.
type Response struct {
	Data   json.RawMessage `json:"data,omitempty"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLError represents a single GraphQL error.
type GraphQLError struct {
	Message string `json:"message"`
}

// Do executes a GraphQL request against a URL.
func Do(url string, req Request, headers map[string]string, timeout time.Duration) (*Response, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var gqlResp Response
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w (body: %s)", err, string(respBody))
	}

	return &gqlResp, nil
}

// PrettyPrint formats a GraphQL response for display.
func PrettyPrint(resp *Response) string {
	if len(resp.Errors) > 0 {
		var buf bytes.Buffer
		buf.WriteString("Errors:\n")
		for _, e := range resp.Errors {
			buf.WriteString(fmt.Sprintf("  - %s\n", e.Message))
		}
		return buf.String()
	}

	var indented bytes.Buffer
	if err := json.Indent(&indented, resp.Data, "", "  "); err != nil {
		return string(resp.Data)
	}
	return indented.String()
}
