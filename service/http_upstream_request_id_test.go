package service

import (
	"net/http"
	"strings"
	"testing"
)

func TestExtractUpstreamRequestId(t *testing.T) {
	cases := []struct {
		name   string
		header map[string]string
		want   string
	}{
		{"new-api 系上游", map[string]string{"X-Oneapi-Request-Id": "20261112abc"}, "20261112abc"},
		{"sub2api 上游", map[string]string{"x-request-id": "bf8c9172-8222-48cc-8def-b2dbc7ed5e2b"}, "bf8c9172-8222-48cc-8def-b2dbc7ed5e2b"},
		{"Anthropic 官方", map[string]string{"request-id": "req_011CabcDEF"}, "req_011CabcDEF"},
		{"同时存在时 new-api 头优先", map[string]string{"X-Oneapi-Request-Id": "oneapi", "X-Request-Id": "gateway", "Request-Id": "req_x"}, "oneapi"},
		{"X-Request-Id 优先于 Request-Id", map[string]string{"X-Request-Id": "gateway", "Request-Id": "req_x"}, "gateway"},
		{"空白值跳过，取下一个", map[string]string{"X-Request-Id": "   ", "Request-Id": "req_x"}, "req_x"},
		{"都没有", map[string]string{"Content-Type": "application/json"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.header {
				h.Set(k, v)
			}
			if got := ExtractUpstreamRequestId(h); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractUpstreamRequestIdTruncatesToColumnWidth(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", strings.Repeat("a", 300))
	got := ExtractUpstreamRequestId(h)
	if len(got) != upstreamRequestIdMaxLen {
		t.Fatalf("len = %d, want %d", len(got), upstreamRequestIdMaxLen)
	}
}
