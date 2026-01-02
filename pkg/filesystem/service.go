package filesystem

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

// BrowseOptions has the same structure as BrowseQuery to allow direct type conversion.
type BrowseOptions BrowseQuery

func (s *Service) Browse(opts BrowseOptions) (*BrowseResponse, error) {
	// Default path to root.
	path := opts.Path
	if path == "" {
		path = "/"
	}

	// Resolve to absolute path and clean it.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// Resolve symlinks to prevent directory traversal.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If the path doesn't exist or we can't resolve it, use the absolute path.
		realPath = absPath
	}

	// Check if path exists and is a directory.
	info, err := os.Stat(realPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrInvalid
	}

	// Read directory entries.
	dirEntries, err := os.ReadDir(realPath)
	if err != nil {
		return nil, err
	}

	// Filter and collect entries.
	var entries []Entry
	for _, de := range dirEntries {
		name := de.Name()

		// Skip hidden files/directories unless requested.
		if !opts.ShowHidden && strings.HasPrefix(name, ".") {
			continue
		}

		// Apply search filter (case-insensitive).
		if opts.Search != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(opts.Search)) {
			continue
		}

		entryPath := filepath.Join(realPath, name)
		entries = append(entries, Entry{
			Name:  name,
			Path:  entryPath,
			IsDir: de.IsDir(),
		})
	}

	// Sort: directories first (alphabetically), then files (alphabetically).
	sort.Slice(entries, func(i, j int) bool {
		// Directories come before files.
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		// Alphabetical within same type.
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	total := len(entries)

	// Apply pagination.
	start := opts.Offset
	if start > total {
		start = total
	}
	end := start + opts.Limit
	if end > total {
		end = total
	}
	paginatedEntries := entries[start:end]
	hasMore := end < total

	// Calculate parent path.
	parentPath := ""
	if realPath != "/" {
		parentPath = filepath.Dir(realPath)
	}

	return &BrowseResponse{
		CurrentPath: realPath,
		ParentPath:  parentPath,
		Entries:     paginatedEntries,
		Total:       total,
		HasMore:     hasMore,
	}, nil
}
