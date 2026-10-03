package httpserver

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Loopayeh/pkg-sender/nas/internal/ps5"
)

// InstalledChecker is optionally implemented by installers that can ask the
// PS5 receiver whether a title is really installed.
type InstalledChecker interface {
	Installed(ctx context.Context, titleID string) (bool, error)
}

const (
	installedTTL     = 20 * time.Second
	installedFailTTL = 10 * time.Second
	installedTimeout = 6 * time.Second
	installedWorkers = 4
)

var titleIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// installedSnapshot is the /api/installed reply. Supported is false when the
// receiver is unreachable or too old, in which case clients keep using the
// install history.
type installedSnapshot struct {
	Supported bool            `json:"supported"`
	Titles    map[string]bool `json:"titles"`
	Error     string          `json:"error,omitempty"`
	CheckedAt time.Time       `json:"checkedAt"`
}

type installedCache struct {
	mu      sync.Mutex
	snap    installedSnapshot
	expires time.Time
}

// gameTitleIDs returns the distinct title ids of base-game packages. Patches
// and DLC share or extend a game's title id, so the receiver's title-level
// answer says nothing about them.
func (s *Server) gameTitleIDs() []string {
	seen := map[string]bool{}
	var ids []string
	for _, pkg := range s.store.List() {
		if pkg.PackageType != "game" {
			continue
		}
		id := strings.ToUpper(strings.TrimSpace(pkg.TitleID))
		if !titleIDPattern.MatchString(id) || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func (s *Server) installedSnapshot(ctx context.Context) installedSnapshot {
	s.installed.mu.Lock()
	defer s.installed.mu.Unlock()
	if time.Now().Before(s.installed.expires) {
		return s.installed.snap
	}

	snap := installedSnapshot{Titles: map[string]bool{}, CheckedAt: time.Now().UTC()}
	ttl := installedTTL
	checker, ok := s.installer.(InstalledChecker)
	if !ok {
		snap.Error = "installer does not support installed-title queries"
		ttl = installedFailTTL
	} else {
		ids := s.gameTitleIDs()
		cctx, cancel := context.WithTimeout(ctx, installedTimeout)
		defer cancel()

		type result struct {
			id        string
			installed bool
			err       error
		}
		jobs := make(chan string)
		results := make(chan result, len(ids))
		var wg sync.WaitGroup
		for w := 0; w < installedWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range jobs {
					installed, err := checker.Installed(cctx, id)
					results <- result{id, installed, err}
				}
			}()
		}
		for _, id := range ids {
			jobs <- id
		}
		close(jobs)
		wg.Wait()
		close(results)

		var firstErr error
		for r := range results {
			if r.err != nil {
				if firstErr == nil {
					firstErr = r.err
				}
				continue
			}
			snap.Titles[r.id] = r.installed
		}
		// All-or-nothing: a partial answer would flip some games back to
		// history-based status, which is more confusing than none.
		if firstErr != nil {
			snap.Titles = map[string]bool{}
			snap.Error = firstErr.Error()
			if errors.Is(firstErr, ps5.ErrInstalledUnsupported) {
				snap.Error = "PS5 receiver is too old for installed-title queries (needs build 20261003-03 or newer)"
			}
			ttl = installedFailTTL
		} else {
			snap.Supported = true
		}
	}

	s.installed.snap = snap
	s.installed.expires = time.Now().Add(ttl)
	return snap
}

func (s *Server) handleInstalled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	writeJSON(w, http.StatusOK, s.installedSnapshot(r.Context()))
}
