package bilibiliutil

import (
	"context"
	"errors"
	coretypes "github.com/difyz9/ytb2bili/internal/core/types"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSubtitleReadOnlyFailover(t *testing.T) {
	for _, tt := range []struct {
		name, method string
		err          error
		code         int
		cancel, want bool
	}{
		{"GET EOF", "GET", io.EOF, 0, false, true},
		{"HEAD EOF", "HEAD", io.ErrUnexpectedEOF, 0, false, true},
		{"TLS timeout", "GET", context.DeadlineExceeded, 0, false, true},
		{"POST is never replayed", "POST", io.EOF, 0, false, false},
		{"HTTP rejection is not retried", "GET", nil, 403, false, false},
		{"success", "GET", nil, 200, false, false},
		{"non network error", "GET", errors.New("bad request"), 0, false, false},
		{"cancelled request", "GET", io.EOF, 0, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			req, _ := http.NewRequest(tt.method, "https://member.bilibili.com/test?bvid=example", nil)
			req.Header.Set("Cookie", "test-cookie")
			if tt.cancel {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			tr := &subtitleTransport{
				direct: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if tt.err != nil {
						return nil, tt.err
					}
					return &http.Response{StatusCode: tt.code, Body: http.NoBody}, nil
				}),
				fallback: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					called = true
					if r.URL.String() != req.URL.String() || r.Header.Get("Cookie") != "test-cookie" {
						t.Fatal("request changed")
					}
					return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
				}),
			}
			resp, _ := tr.RoundTrip(req)
			if resp != nil {
				resp.Body.Close()
			}
			if called != tt.want {
				t.Fatalf("fallback=%v want=%v", called, tt.want)
			}
		})
	}
}
func TestSubtitleClientUsesDirectTransport(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	client, err := NewSubtitleClient(&coretypes.AppConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.GetHTTPClient().CloseIdleConnections()
	tr := client.GetHTTPClient().Transport.(*subtitleTransport)
	direct := tr.direct.(*http.Transport)
	if direct.Proxy != nil || tr.fallback != nil || direct.TLSHandshakeTimeout != 30*time.Second {
		t.Fatal("unexpected subtitle transport")
	}
}
