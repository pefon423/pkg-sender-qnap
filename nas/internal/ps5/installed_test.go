package ps5

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientInstalled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("id") {
		case "PPSA32236":
			_, _ = w.Write([]byte(`{"installed":true}`))
		case "PPSA26791":
			_, _ = w.Write([]byte(`{"installed":false}`))
		default:
			_, _ = w.Write([]byte("error:bad id"))
		}
	}))
	defer srv.Close()
	c, err := NewWithBaseURL(srv.URL, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got, err := c.Installed(ctx, "PPSA32236"); err != nil || !got {
		t.Fatalf("installed title: got=%v err=%v", got, err)
	}
	if got, err := c.Installed(ctx, "PPSA26791"); err != nil || got {
		t.Fatalf("missing title: got=%v err=%v", got, err)
	}
	// An old receiver answers 200 with something that is not the JSON reply.
	if _, err := c.Installed(ctx, "X/Y"); !errors.Is(err, ErrInstalledUnsupported) {
		t.Fatalf("old receiver: err=%v, want ErrInstalledUnsupported", err)
	}
}
