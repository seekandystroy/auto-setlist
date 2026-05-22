package adapters

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCallbackHandler_HappyPath(t *testing.T) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	h := callbackHandler("expected-state", codeCh, errCh)

	req := httptest.NewRequest(http.MethodGet, "/spotify_callback?code=abc123&state=expected-state", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	select {
	case code := <-codeCh:
		if code != "abc123" {
			t.Errorf("expected code abc123, got %s", code)
		}
	default:
		t.Fatal("expected code on channel")
	}
}

func TestCallbackHandler_StateMismatch(t *testing.T) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	h := callbackHandler("expected-state", codeCh, errCh)

	req := httptest.NewRequest(http.MethodGet, "/spotify_callback?code=abc123&state=wrong-state", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error on channel")
		}
	default:
		t.Fatal("expected error on channel")
	}
}

func TestCallbackHandler_MissingCode(t *testing.T) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	h := callbackHandler("expected-state", codeCh, errCh)

	req := httptest.NewRequest(http.MethodGet, "/spotify_callback?state=expected-state", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error on channel")
		}
	default:
		t.Fatal("expected error on channel")
	}
}

func TestWaitForCode_Timeout(t *testing.T) {
	a := NewSpotifyCallbackAdapter()

	// Use a free port to avoid conflicts.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not allocate port: %v", err)
	}
	a.addr = ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	a.timeout = 50 * time.Millisecond

	_, err = a.WaitForCode(ctx, "some-state")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForCode_Integration(t *testing.T) {
	a := NewSpotifyCallbackAdapter()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not allocate port: %v", err)
	}
	a.addr = ln.Addr().String()
	ln.Close()

	done := make(chan struct{})
	var gotCode string
	var gotErr error

	go func() {
		gotCode, gotErr = a.WaitForCode(context.Background(), "test-state")
		close(done)
	}()

	// Give the server a moment to start.
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get("http://" + a.addr + "/spotify_callback?code=mycode&state=test-state")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WaitForCode to return")
	}

	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if gotCode != "mycode" {
		t.Errorf("expected mycode, got %s", gotCode)
	}
}
