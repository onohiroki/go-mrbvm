package bundle

import (
	"io/fs"
	"path"
	"time"

	"github.com/onohiroki/go-mrbvm/mrbvm"
)

// mrbvmSourceFS adapts a Bundle to mrbvm.SourceFS.
type mrbvmSourceFS struct {
	bundle *Bundle
}

func (f mrbvmSourceFS) ReadFile(name string) ([]byte, error) {
	return f.bundle.ReadFile(name)
}

func (f mrbvmSourceFS) Stat(name string) (fs.FileInfo, error) {
	data, err := f.bundle.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return bundleFileInfo{name: path.Base(name), size: int64(len(data))}, nil
}

type bundleFileInfo struct {
	name string
	size int64
}

func (i bundleFileInfo) Name() string       { return i.name }
func (i bundleFileInfo) Size() int64        { return i.size }
func (i bundleFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i bundleFileInfo) ModTime() time.Time { return time.Unix(0, 0).UTC() }
func (i bundleFileInfo) IsDir() bool        { return false }
func (i bundleFileInfo) Sys() any           { return nil }

var _ mrbvm.SourceFS = mrbvmSourceFS{}
