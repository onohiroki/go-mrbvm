// Package bundle implements the .mrbb application archive and the runtime
// that executes one. An archive is a ZIP file containing a manifest followed
// by precompiled RITE 04.00 bytecode files. It is produced by gomrbpack.
package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// FormatVersion is the manifest format written by this package.
const FormatVersion = 1

// ManifestName is the archive member that describes the bundle.
const ManifestName = "manifest.json"

// Extension is the conventional file extension for a bundle.
const Extension = ".mrbb"

// Member kinds recorded in the manifest.
const (
	// KindSource is Ruby source.
	KindSource = "rb"
	// KindBytecode is RITE 04.00 bytecode.
	KindBytecode = "mrb"
	// KindData is an opaque asset read through File.
	KindData = "data"
)

const (
	maxBundleSize = 256 << 20
	maxFileSize   = 64 << 20
)

// zipMagic is the local file header signature.
var zipMagic = []byte("PK\x03\x04")

// File describes one archive member.
type File struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest is the JSON document stored at the head of an archive.
type Manifest struct {
	Format    int      `json:"format"`
	Entry     string   `json:"entry"`
	LoadPaths []string `json:"load_paths,omitempty"`
	Files     []File   `json:"files"`
}

// Entry is one input file to Write. Kind is optional; an empty Kind is
// derived from the data (RITE bytecode or Ruby source).
type Entry struct {
	Path string
	Kind string
	Data []byte
}

// Write encodes a bundle to w. entry names the archive member to execute and
// must be present in entries. Paths are sorted so the same inputs always
// produce the same archive bytes.
func Write(w io.Writer, entry string, loadPaths []string, entries []Entry) error {
	if len(entries) == 0 {
		return errors.New("bundle: no files")
	}
	cleaned, err := cleanEntryName(entry)
	if err != nil {
		return fmt.Errorf("bundle: entry: %w", err)
	}
	entry = cleaned

	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	for i := range sorted {
		clean, err := cleanEntryName(sorted[i].Path)
		if err != nil {
			return fmt.Errorf("bundle: %s: %w", sorted[i].Path, err)
		}
		if clean != sorted[i].Path {
			return fmt.Errorf("bundle: %s: path is not normalized (%s)", sorted[i].Path, clean)
		}
		if len(sorted[i].Data) > maxFileSize {
			return fmt.Errorf("bundle: %s: file is too large", sorted[i].Path)
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Path == sorted[i-1].Path {
			return fmt.Errorf("bundle: duplicate file %s", sorted[i].Path)
		}
	}

	manifest := Manifest{
		Format:    FormatVersion,
		Entry:     entry,
		LoadPaths: make([]string, 0, len(loadPaths)),
	}
	foundEntry := false
	entryKind := ""
	for _, item := range sorted {
		sum := sha256.Sum256(item.Data)
		kind := item.Kind
		if kind == "" {
			kind = KindSource
			if bytes.HasPrefix(item.Data, []byte("RITE")) {
				kind = KindBytecode
			}
		}
		if err := validateKind(kind, item.Data); err != nil {
			return fmt.Errorf("bundle: %s: %w", item.Path, err)
		}
		manifest.Files = append(manifest.Files, File{
			Path:   item.Path,
			Kind:   kind,
			SHA256: hex.EncodeToString(sum[:]),
			Size:   int64(len(item.Data)),
		})
		if item.Path == entry {
			foundEntry = true
			entryKind = kind
		}
	}
	if !foundEntry {
		return fmt.Errorf("bundle: entry %s is not one of the input files", entry)
	}
	if entryKind == KindData {
		return fmt.Errorf("bundle: entry %s is a data file, not executable code", entry)
	}
	for _, loadPath := range loadPaths {
		clean, err := cleanDirName(loadPath)
		if err != nil {
			return fmt.Errorf("bundle: load path %q: %w", loadPath, err)
		}
		manifest.LoadPaths = append(manifest.LoadPaths, clean)
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	zw := zip.NewWriter(w)
	if err := writeMember(zw, ManifestName, manifestJSON, zip.Store); err != nil {
		return err
	}
	for _, item := range sorted {
		if err := writeMember(zw, item.Path, item.Data, zip.Deflate); err != nil {
			return err
		}
	}
	return zw.Close()
}

func writeMember(zw *zip.Writer, name string, data []byte, method uint16) error {
	header := &zip.FileHeader{Name: name, Method: method}
	header.SetMode(0o644)
	header.Modified = time.Unix(0, 0).UTC()
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// Bundle is an opened archive: a manifest plus its decoded members.
type Bundle struct {
	manifest Manifest
	files    map[string][]byte
	code     map[string][]byte
	assets   map[string][]byte
}

// Open decodes an in-memory archive.
func Open(data []byte) (*Bundle, error) {
	if len(data) > maxBundleSize {
		return nil, fmt.Errorf("bundle: archive is too large (%d bytes)", len(data))
	}
	if !IsArchive(data) {
		return nil, errors.New("bundle: not a .mrbb archive")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}

	var manifest []byte
	total := 0
	members := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("bundle: %s: %w", file.Name, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxFileSize+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("bundle: %s: %w", file.Name, err)
		}
		if len(data) > maxFileSize {
			return nil, fmt.Errorf("bundle: %s: file is too large", file.Name)
		}
		total += len(data)
		if total > maxBundleSize {
			return nil, errors.New("bundle: uncompressed contents are too large")
		}
		if file.Name == ManifestName {
			if manifest != nil {
				return nil, fmt.Errorf("bundle: duplicate %s", ManifestName)
			}
			manifest = data
			continue
		}
		if _, exists := members[file.Name]; exists {
			return nil, fmt.Errorf("bundle: duplicate member %s", file.Name)
		}
		members[file.Name] = data
	}
	if manifest == nil {
		return nil, fmt.Errorf("bundle: missing %s", ManifestName)
	}

	var m Manifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, fmt.Errorf("bundle: manifest: %w", err)
	}
	if m.Format <= 0 || m.Format > FormatVersion {
		return nil, fmt.Errorf("bundle: unsupported format %d", m.Format)
	}
	if m.Entry == "" {
		return nil, errors.New("bundle: manifest has no entry")
	}
	if _, ok := members[m.Entry]; !ok {
		return nil, fmt.Errorf("bundle: entry %s is missing", m.Entry)
	}

	code := make(map[string][]byte)
	assets := make(map[string][]byte)
	listed := make(map[string]bool, len(m.Files))
	for _, file := range m.Files {
		if _, err := cleanEntryName(file.Path); err != nil {
			return nil, fmt.Errorf("bundle: manifest file %q: %w", file.Path, err)
		}
		listed[file.Path] = true
		data, ok := members[file.Path]
		if !ok {
			return nil, fmt.Errorf("bundle: manifest file %s is missing", file.Path)
		}
		if int64(len(data)) != file.Size {
			return nil, fmt.Errorf("bundle: %s: size mismatch", file.Path)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != file.SHA256 {
			return nil, fmt.Errorf("bundle: %s: checksum mismatch", file.Path)
		}
		if err := validateKind(file.Kind, data); err != nil {
			return nil, fmt.Errorf("bundle: %s: %w", file.Path, err)
		}
		if file.Kind == KindData {
			assets[file.Path] = data
		} else {
			code[file.Path] = data
		}
	}
	if _, ok := code[m.Entry]; !ok {
		return nil, fmt.Errorf("bundle: entry %s is not executable code", m.Entry)
	}
	for name := range members {
		if !listed[name] {
			return nil, fmt.Errorf("bundle: unlisted member %s", name)
		}
	}
	for i, loadPath := range m.LoadPaths {
		clean, err := cleanDirName(loadPath)
		if err != nil {
			return nil, fmt.Errorf("bundle: load path %q: %w", loadPath, err)
		}
		m.LoadPaths[i] = clean
	}

	return &Bundle{manifest: m, files: members, code: code, assets: assets}, nil
}

func validateKind(kind string, data []byte) error {
	switch kind {
	case KindSource, KindData:
		return nil
	case KindBytecode:
		if !bytes.HasPrefix(data, []byte("RITE")) {
			return errors.New("bytecode member does not start with RITE")
		}
		return nil
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}

// OpenFile reads and decodes an archive from disk.
func OpenFile(name string) (*Bundle, error) {
	info, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("bundle: %s is a directory", name)
	}
	if info.Size() > maxBundleSize {
		return nil, fmt.Errorf("bundle: %s: archive is too large", name)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return Open(data)
}

// IsArchive reports whether data begins like a ZIP file. It is a cheap
// pre-check; Open performs the full validation.
func IsArchive(data []byte) bool {
	return len(data) >= len(zipMagic) && bytes.Equal(data[:len(zipMagic)], zipMagic)
}

// Manifest returns a copy of the manifest.
func (b *Bundle) Manifest() Manifest {
	m := b.manifest
	m.Files = append([]File(nil), b.manifest.Files...)
	m.LoadPaths = append([]string(nil), b.manifest.LoadPaths...)
	return m
}

// Entry returns the archive member that Run executes.
func (b *Bundle) Entry() string { return b.manifest.Entry }

// Files returns the sorted archive member names.
func (b *Bundle) Files() []string {
	names := make([]string, 0, len(b.files))
	for name := range b.files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReadFile returns a member's bytes.
func (b *Bundle) ReadFile(name string) ([]byte, error) {
	clean, err := cleanEntryName(name)
	if err != nil {
		return nil, err
	}
	data, ok := b.files[clean]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

// Extract writes every member below dir, creating directories as needed.
func (b *Bundle) Extract(dir string) error {
	for _, name := range b.Files() {
		target := path.Join(dir, name)
		if err := os.MkdirAll(path.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b.files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SourceFS returns a read-only view of the executable members for
// mrbvm.Options.SourceFS. Data members are not visible here.
func (b *Bundle) SourceFS() mrbvm.SourceFS {
	return memberFS{members: b.code}
}

// AssetFS returns a read-only view of the data members for
// mrbvm.Options.AssetFS.
func (b *Bundle) AssetFS() mrbvm.SourceFS {
	return memberFS{members: b.assets}
}

// CleanPath normalizes and validates a path relative to the bundle root.
func CleanPath(name string) (string, error) {
	return cleanEntryName(name)
}

func cleanEntryName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") {
		return "", errors.New("invalid path")
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", errors.New("invalid path")
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes the bundle root")
	}
	if strings.ContainsRune(clean, 0) {
		return "", errors.New("invalid path")
	}
	return clean, nil
}

func cleanDirName(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSuffix(strings.TrimPrefix(name, "./"), "/")
	if name == "" || name == "." {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		return "", errors.New("invalid path")
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes the bundle root")
	}
	return clean, nil
}
