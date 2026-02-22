package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Request holds the parameters for an HTTP request.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
	Timeout time.Duration
	Verbose bool
}

// Response holds the result of an HTTP request.
type Response struct {
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Duration   time.Duration     `json:"duration"`
}

// Do executes an HTTP request and returns a structured response.
func Do(req Request) (*Response, error) {
	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}

	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: req.Timeout}

	start := time.Now()
	resp, err := client.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
		Body:       string(body),
		Duration:   elapsed,
	}, nil
}

// PrintResponse displays an HTTP response to stdout.
func PrintResponse(resp *Response, verbose bool) {
	if verbose {
		fmt.Fprintf(os.Stderr, "Status: %s\n", resp.Status)
		fmt.Fprintf(os.Stderr, "Duration: %s\n", resp.Duration.Round(time.Millisecond))
		fmt.Fprintln(os.Stderr, "")
		for k, v := range resp.Headers {
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
		}
		fmt.Fprintln(os.Stderr, "")
	}

	// Try to pretty-print JSON.
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(resp.Body), "", "  "); err == nil {
		fmt.Println(buf.String())
	} else {
		fmt.Println(resp.Body)
	}
}
