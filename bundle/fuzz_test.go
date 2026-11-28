package bundle_test

import (
	"bytes"
	"path"
	"strings"
	"testing"

	"github.com/onohiroki/go-mrbvm/bundle"
)

func seedArchive(t testing.TB) []byte {
	t.Helper()
	var buf bytes.Buffer
	err := bundle.Write(&buf, "main.rb", []string{"lib"}, []bundle.Entry{
		{Path: "main.rb", Data: []byte("puts 1\n")},
		{Path: "lib/data.txt", Data: []byte("data\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func FuzzOpen(f *testing.F) {
	f.Add(seedArchive(f))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte("not a zip"))
	f.Fuzz(func(t *testing.T, data []byte) {
		// Keep fuzz cases small; the production limits are covered by unit
		// tests. This avoids spending the fuzz budget on allocation limits.
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _ = bundle.Open(data)
	})
}

func FuzzCleanPath(f *testing.F) {
	for _, seed := range []string{"main.rb", "lib/a.mrb", "../x", "/x", "./a", `a\b`, "a/../b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		clean, err := bundle.CleanPath(name)
		if err != nil {
			return
		}
		if clean == "" || clean == "." || clean == ".." {
			t.Fatalf("CleanPath(%q) returned %q", name, clean)
		}
		if path.IsAbs(clean) || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
			t.Fatalf("CleanPath(%q) escaped the root: %q", name, clean)
		}
	})
}
