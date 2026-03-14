package http

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

type mockBaseTransport struct {
	resp *http.Response
	err  error
}

func (m *mockBaseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.resp, m.err
}

func TestTransport_RoundTrip(t *testing.T) {
	t.Run("BaseError", func(t *testing.T) {
		tr := &Transport{
			base: &mockBaseTransport{err: io.EOF},
		}
		req := httptest.NewRequest("GET", "http://example.com", nil)
		if _, err := tr.RoundTrip(req); err == nil {
			t.Fatal("expected base transport error")
		}
	})

	t.Run("Brotli_Decompression", func(t *testing.T) {
		var buf bytes.Buffer
		bw := brotli.NewWriter(&buf)
		bw.Write([]byte("uncompressed"))
		bw.Close()

		resp := &http.Response{
			Header: make(http.Header),
			Body:   io.NopCloser(&buf),
		}
		resp.Header.Set("Content-Encoding", "br")
		resp.Header.Set("Content-Length", "999")

		tr := &Transport{
			base: &mockBaseTransport{resp: resp},
		}

		req := httptest.NewRequest("GET", "http://example.com", nil)
		gotResp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip failed: %v", err)
		}

		body, _ := io.ReadAll(gotResp.Body)
		if string(body) != "uncompressed" {
			t.Errorf("expected 'uncompressed', got %q", string(body))
		}
		if gotResp.Header.Get("Content-Encoding") != "" {
			t.Errorf("expected Content-Encoding to be cleared")
		}
		if gotResp.ContentLength != -1 {
			t.Errorf("expected ContentLength -1, got %d", gotResp.ContentLength)
		}
	})

	t.Run("No_Compression", func(t *testing.T) {
		resp := &http.Response{
			Header: make(http.Header),
			Body:   io.NopCloser(strings.NewReader("plain")),
		}
		tr := &Transport{
			base: &mockBaseTransport{resp: resp},
		}
		req := httptest.NewRequest("GET", "http://example.com", nil)
		gotResp, _ := tr.RoundTrip(req)
		body, _ := io.ReadAll(gotResp.Body)
		if string(body) != "plain" {
			t.Errorf("expected 'plain', got %q", string(body))
		}
	})
}
