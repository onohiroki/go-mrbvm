package mrbvm

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// SourceFS is the read-only filesystem the VM uses to load Ruby programs for
// load, require, and require_relative. A nil SourceFS selects the host
// filesystem with the same path semantics as mruby.
//
// Implementations are expected to be safe for the goroutine that owns the
// State. NewSourceFS adapts an io/fs.FS such as embed.FS or fstest.MapFS.
type SourceFS interface {
	ReadFile(name string) ([]byte, error)
	Stat(name string) (fs.FileInfo, error)
}

type fsSource struct {
	fsys fs.FS
}

// NewSourceFS adapts fsys to SourceFS. Names use forward slashes and are
// resolved relative to the root of fsys; absolute paths and paths that escape
// the root are rejected.
func NewSourceFS(fsys fs.FS) SourceFS {
	return &fsSource{fsys: fsys}
}

func cleanFSPath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") {
		return "", fs.ErrInvalid
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", fs.ErrInvalid
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fs.ErrInvalid
	}
	return clean, nil
}

func (f *fsSource) ReadFile(name string) ([]byte, error) {
	clean, err := cleanFSPath(name)
	if err != nil {
		return nil, err
	}
	return fs.ReadFile(f.fsys, clean)
}

func (f *fsSource) Stat(name string) (fs.FileInfo, error) {
	clean, err := cleanFSPath(name)
	if err != nil {
		return nil, err
	}
	return fs.Stat(f.fsys, clean)
}

// readAsset returns a bundled asset. The second result reports whether the
// asset exists; errors (including invalid paths) fall through to the host.
func (s *State) readAsset(name string) ([]byte, bool) {
	if s.assetFS == nil {
		return nil, false
	}
	data, err := s.assetFS.ReadFile(name)
	if err != nil {
		return nil, false
	}
	return data, true
}

func (s *State) assetStat(name string) (fs.FileInfo, bool) {
	if s.assetFS == nil {
		return nil, false
	}
	info, err := s.assetFS.Stat(name)
	if err != nil {
		return nil, false
	}
	return info, true
}

func (s *State) sourceReadFile(name string) ([]byte, error) {
	if s.sourceFS == nil {
		return os.ReadFile(name)
	}
	return s.sourceFS.ReadFile(name)
}

func (s *State) sourceStat(name string) (fs.FileInfo, error) {
	if s.sourceFS == nil {
		return os.Stat(name)
	}
	return s.sourceFS.Stat(name)
}

func (s *State) sourceExt(name string) string {
	if s.sourceFS == nil {
		return filepath.Ext(name)
	}
	return path.Ext(name)
}

func (s *State) sourceDir(name string) string {
	if s.sourceFS == nil {
		return filepath.Dir(name)
	}
	return path.Dir(name)
}

func (s *State) sourceJoin(base, name string) string {
	if s.sourceFS == nil {
		return filepath.Join(base, name)
	}
	return path.Join(base, name)
}

func (s *State) canonicalSourcePath(name string) string {
	if s.sourceFS == nil {
		if absolute, err := filepath.Abs(name); err == nil {
			return absolute
		}
		return name
	}
	return path.Clean(name)
}
