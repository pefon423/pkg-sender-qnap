package httpserver

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Loopayeh/pkg-sender/nas/internal/pkgstore"
)

func TestInstalledEndpointWithoutSupport(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "A.pkg"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := pkgstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Scan(); err != nil {
		t.Fatal(err)
	}
	// fakeInstaller has no Installed method, like an installer that cannot query the PS5.
	app, err := New(store, &fakeInstaller{}, "http://192.168.1.20:9898", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/installed")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Supported bool            `json:"supported"`
		Titles    map[string]bool `json:"titles"`
		Error     string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || got.Supported || got.Titles == nil || got.Error == "" {
		t.Fatalf("status=%d reply=%+v, want 200, unsupported, empty titles map, an error text", resp.StatusCode, got)
	}
}
