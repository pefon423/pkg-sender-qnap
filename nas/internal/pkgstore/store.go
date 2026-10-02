package pkgstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Loopayeh/pkg-sender/nas/internal/pkgmeta"
)

type Package struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	RelativePath   string `json:"relativePath"`
	Path           string `json:"path,omitempty"`
	LibraryRoot    string `json:"libraryRoot,omitempty"`
	Size           int64  `json:"size"`
	MetadataParsed bool   `json:"metadataParsed"`
	// ModifiedAt lets clients tell a file replaced in place (same path, so
	// same ID) from the version they installed earlier.
	ModifiedAt time.Time `json:"modifiedAt"`
	pkgmeta.Metadata
}

type entry struct {
	Package
	path    string
	modTime time.Time
}

type Store struct {
	root    string
	roots   []string
	aliases TitleAliases

	mu      sync.RWMutex
	entries map[string]entry
	list    []Package
}

func New(root string) (*Store, error) {
	return NewWithTitleAliases(root, nil)
}

func NewWithTitleAliases(root string, aliases TitleAliases) (*Store, error) {
	return NewWithRootsAndTitleAliases([]string{root}, aliases)
}

func NewWithRoots(roots []string) (*Store, error) {
	return NewWithRootsAndTitleAliases(roots, nil)
}

func NewWithRootsAndTitleAliases(roots []string, aliases TitleAliases) (*Store, error) {
	normalized, err := NormalizeRoots(roots)
	if err != nil {
		return nil, err
	}
	return &Store{
		root:    normalized[0],
		roots:   normalized,
		aliases: aliases.normalized(),
		entries: make(map[string]entry),
	}, nil
}

// rootAccessTimeout bounds how long a single root's existence check (or, in
// Scan, a single root's directory walk) may take. Without it, an
// unreachable or newly-unresponsive network path (a UNC share, a mounted
// remote volume) can block server startup or a rescan indefinitely, since
// neither os.Stat nor filepath.WalkDir accept a deadline.
const rootAccessTimeout = 10 * time.Second

func NormalizeRoots(roots []string) ([]string, error) {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		// A root that is not accessible right now -- including one that
		// times out, e.g. a network share that has stopped responding --
		// is skipped rather than failing every other configured root too.
		// Mirrors scanRoot's existing "one unreadable subtree does not
		// invalidate the whole library" behavior, one level up.
		info, err := statWithTimeout(abs, rootAccessTimeout)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[abs] = true
		normalized = append(normalized, abs)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one package root is required (none of the configured roots are currently accessible)")
	}
	return normalized, nil
}

// statWithTimeout is os.Stat with an upper bound on how long it may block.
// os.Stat itself has no cancellation, so on timeout the underlying call may
// still be outstanding in the background; the caller simply stops waiting
// on it and treats the root as inaccessible for now.
func statWithTimeout(path string, timeout time.Duration) (os.FileInfo, error) {
	type result struct {
		info os.FileInfo
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		info, err := os.Stat(path)
		resultCh <- result{info: info, err: err}
	}()
	select {
	case r := <-resultCh:
		return r.info, r.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out after %s waiting for %q to respond", timeout, path)
	}
}

func (s *Store) Root() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root
}

func (s *Store) Roots() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.roots))
	copy(out, s.roots)
	return out
}

func (s *Store) SetRoots(roots []string) error {
	normalized, err := NormalizeRoots(roots)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.root = normalized[0]
	s.roots = normalized
	s.mu.Unlock()
	return nil
}

func (s *Store) Scan() (int, error) {
	s.mu.RLock()
	roots := make([]string, len(s.roots))
	copy(roots, s.roots)
	aliases := s.aliases
	s.mu.RUnlock()

	next := make(map[string]entry)
	packages := make([]Package, 0)
	for rootIndex, root := range roots {
		if err := scanRootWithTimeout(rootIndex, root, len(roots), aliases, next, &packages, rootAccessTimeout); err != nil {
			return 0, err
		}
	}

	sort.Slice(packages, func(i, j int) bool {
		left, right := strings.ToLower(packages[i].Path), strings.ToLower(packages[j].Path)
		if left != right {
			return left < right
		}
		return packages[i].ID < packages[j].ID
	})

	s.mu.Lock()
	s.entries = next
	s.list = packages
	s.mu.Unlock()

	return len(packages), nil
}

// scanRootWithTimeout runs scanRoot but never blocks the caller longer than
// timeout. filepath.WalkDir cannot be cancelled mid-flight, so a root that
// does not finish in time (e.g. a network share that stops responding
// partway through) has its goroutine abandoned rather than killed: it
// writes only into its own local next/packages, merged into the caller's
// shared next/packages only if it completes in time, so an abandoned scan
// can never race with the caller's data structures. A timeout is not
// treated as a hard error; other roots still get their full budget.
func scanRootWithTimeout(rootIndex int, root string, rootCount int, aliases TitleAliases, next map[string]entry, packages *[]Package, timeout time.Duration) error {
	type result struct {
		next     map[string]entry
		packages []Package
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		localNext := make(map[string]entry)
		localPackages := make([]Package, 0)
		err := scanRoot(rootIndex, root, rootCount, aliases, localNext, &localPackages)
		resultCh <- result{next: localNext, packages: localPackages, err: err}
	}()
	select {
	case r := <-resultCh:
		if r.err != nil {
			return r.err
		}
		for id, e := range r.next {
			next[id] = e
		}
		*packages = append(*packages, r.packages...)
		return nil
	case <-time.After(timeout):
		return nil
	}
}

func scanRoot(rootIndex int, root string, rootCount int, aliases TitleAliases, next map[string]entry, packages *[]Package) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Keep one unreadable subtree from invalidating the whole library.
			if path == root {
				return walkErr
			}
			return nil
		}
		if path == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".pkg") {
			return nil
		}

		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		idInput := rel
		if rootCount > 1 && rootIndex > 0 {
			idInput = strconv.Itoa(rootIndex) + "\x00" + rel
		}
		id := packageID(idInput)
		meta, metaErr := pkgmeta.ReadFile(path)
		if metaErr == nil {
			aliases.apply(&meta)
		}
		pkg := Package{
			ID:             id,
			Name:           filepath.Base(path),
			RelativePath:   rel,
			Path:           filepath.ToSlash(filepath.Join(root, rel)),
			LibraryRoot:    filepath.ToSlash(root),
			Size:           info.Size(),
			ModifiedAt:     info.ModTime(),
			MetadataParsed: metaErr == nil,
			Metadata:       meta,
		}
		next[id] = entry{
			Package: pkg,
			path:    path,
			modTime: info.ModTime(),
		}
		*packages = append(*packages, pkg)
		return nil
	})
}

func (s *Store) List() []Package {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Package, len(s.list))
	copy(out, s.list)
	return out
}

func (s *Store) MissingTitleAliases() []MissingTitleAlias {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byKey := make(map[string]*MissingTitleAlias)
	order := make([]string, 0)
	for _, pkg := range s.list {
		if !pkg.MetadataParsed || strings.TrimSpace(pkg.SecondaryTitle) != "" {
			continue
		}
		key := strings.TrimSpace(pkg.TitleID)
		if key == "" {
			key = strings.TrimSpace(pkg.ContentID)
		}
		if key == "" {
			continue
		}
		key = normalizeAliasKey(key)
		item := byKey[key]
		if item == nil {
			item = &MissingTitleAlias{
				TitleID:            pkg.TitleID,
				ContentID:          pkg.ContentID,
				DisplayTitle:       pkg.DisplayTitle,
				Title:              pkg.Title,
				LocalizedLanguages: localizedLanguageKeys(pkg.LocalizedTitles),
			}
			byKey[key] = item
			order = append(order, key)
		}
		item.PackageCount++
	}
	sort.Strings(order)
	out := make([]MissingTitleAlias, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	return out
}

func (s *Store) TitleAliasExport() TitleAliasExport {
	return BuildTitleAliasExport(s.MissingTitleAliases())
}

func (s *Store) SetTitleAliases(aliases TitleAliases) {
	s.mu.Lock()
	s.aliases = aliases.normalized()
	s.mu.Unlock()
}

func (s *Store) Get(id string) (Package, string, time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.entries[id]
	if !ok {
		return Package{}, "", time.Time{}, false
	}
	return e.Package, e.path, e.modTime, true
}

func packageID(relativePath string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(relativePath)))
	return hex.EncodeToString(sum[:])
}
