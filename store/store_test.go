package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
)

func testApp() fstest.MapFS {
	return fstest.MapFS{
		"cxapp.json": {Data: []byte(`{"name":"t","kind":"console"}`)},
		"main.cx":    {Data: bytes.Repeat([]byte("package main\n"), 30)},
		"lib/a.cx":   {Data: []byte("package main\n")},
		"empty.txt":  {Data: nil},
	}
}

func TestRoundTrip(t *testing.T) {
	m, chunks, err := manifest.Build(testApp(), 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 3 || m.Files[2].Path != "main.cx" || len(m.Files[2].Chunks) != 7 {
		t.Fatalf("files %+v", m.Files)
	}
	dir := t.TempDir()
	hashes, err := WriteBundle(dir, []Built{{m, chunks}})
	if err != nil {
		t.Fatal(err)
	}
	src := FSSource{FS: os.DirFS(dir)}
	apps, errs, err := List(src)
	if err != nil || len(errs) > 0 || len(apps) != 1 || apps[0].Hash != hashes[0] {
		t.Fatal(apps, errs, err)
	}
	files, err := Load(src, apps[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files["main.cx"], testApp()["main.cx"].Data) || len(files["empty.txt"]) != 0 {
		t.Fatal("content differs")
	}

	// A changed chunk is caught.
	c := filepath.Join(dir, ChunkDir, string(m.Files[2].Chunks[3]))
	os.WriteFile(c, bytes.Repeat([]byte("x"), 64), 0o644)
	if _, err := Load(src, apps[0]); err == nil || !strings.Contains(err.Error(), "chunk 3") {
		t.Fatalf("tampered chunk: %v", err)
	}

	// A changed manifest no longer matches its hash.
	mp := filepath.Join(dir, ManifestDir, string(hashes[0])+".json")
	b, _ := os.ReadFile(mp)
	os.WriteFile(mp, bytes.Replace(b, []byte(`"t"`), []byte(`"u"`), 1), 0o644)
	if _, err := Get(src, hashes[0]); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}

func TestBuildDeterministic(t *testing.T) {
	a, _, _ := manifest.Build(testApp(), 0)
	b, _, _ := manifest.Build(testApp(), 0)
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha != hb || !ha.Valid() {
		t.Fatal(ha, hb)
	}
}

func TestBundledAppsVerify(t *testing.T) {
	src := FSSource{FS: os.DirFS("../cmd/cxplayground/bundle")}
	apps, errs, err := List(src)
	if err != nil || len(errs) > 0 || len(apps) == 0 {
		t.Fatal(apps, errs, err)
	}
	for _, a := range apps {
		if _, err := Load(src, a); err != nil {
			t.Error(err)
		}
	}
}
