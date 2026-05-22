package adapters

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const successHTML = `<!DOCTYPE html><html><body>
<h1>Authorization successful</h1>
<p>You may close this tab and return to the terminal.</p>
</body></html>`

const errorHTML = `<!DOCTYPE html><html><body>
<h1>Authorization failed</h1>
<p>%s</p>
</body></html>`

type spotifyCallbackAdapter struct {
	addr    string
	path    string
	timeout time.Duration
}

// This adapter is intended to enable authorization with spotify when running the CLI version
func NewSpotifyCallbackAdapter() *spotifyCallbackAdapter {
	return &spotifyCallbackAdapter{
		addr:    "127.0.0.1:8080",
		path:    "/spotify_callback",
		timeout: 2 * time.Minute,
	}
}

// callbackHandler returns an http.HandlerFunc that validates the state param,
// extracts the code, and signals results via the provided channels.
// Extracted as a standalone function so it can be unit-tested without a live server.
func callbackHandler(expectedState string, codeCh chan<- string, errCh chan<- error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state != expectedState {
			msg := "state mismatch: possible CSRF attack"
			http.Error(w, fmt.Sprintf(errorHTML, msg), http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("spotify callback: %s", msg):
			default:
			}
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			msg := "missing authorization code"
			http.Error(w, fmt.Sprintf(errorHTML, msg), http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("spotify callback: %s", msg):
			default:
			}
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, successHTML)

		select {
		case codeCh <- code:
		default:
		}
	}
}

// WaitForCode starts a local HTTP server on a.addr, waits for Spotify to redirect
// the user to a.path, validates the state parameter, and returns the authorization code.
// The server shuts down after receiving the code or when the context times out.
func (a *spotifyCallbackAdapter) WaitForCode(ctx context.Context, expectedState string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return "", fmt.Errorf("spotify callback: listening on %s: %w", a.addr, err)
	}

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	mux.HandleFunc(a.path, callbackHandler(expectedState, codeCh, errCh))

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			select {
			case errCh <- fmt.Errorf("spotify callback: server error: %w", err):
			default:
			}
		}
	}()

	defer srv.Shutdown(context.Background()) //nolint:errcheck

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("spotify callback: timed out waiting for authorization")
	}
}
