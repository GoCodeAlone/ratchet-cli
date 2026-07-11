//go:build tui_smoke

package main

import (
	"crypto/x509"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCloneSmokeTransportInitializesTLSConfig(t *testing.T) {
	base := &http.Transport{}
	roots := x509.NewCertPool()

	got, err := cloneSmokeTransport(base, roots)
	if err != nil {
		t.Fatalf("clone smoke transport: %v", err)
	}
	if got == base {
		t.Fatal("clone smoke transport returned the caller transport")
	}
	if got.TLSClientConfig == nil || got.TLSClientConfig.RootCAs != roots {
		t.Fatal("clone smoke transport did not install the requested root pool")
	}
	if base.TLSClientConfig != nil && base.TLSClientConfig.RootCAs == roots {
		t.Fatal("clone smoke transport installed roots on the caller transport")
	}
}

func TestCloneSmokeTransportRejectsUnsupportedRoundTripper(t *testing.T) {
	_, err := cloneSmokeTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	}), x509.NewCertPool())
	if err == nil || !strings.Contains(err.Error(), "unsupported default transport") {
		t.Fatalf("clone smoke transport error = %v", err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
