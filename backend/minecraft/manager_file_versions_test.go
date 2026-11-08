package minecraft

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileVersionsKeepNewestFive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "versions")
	for i := 1; i <= 8; i++ {
		if err := writeFileVersion(dir, []byte{byte('0' + i)}, fileVersionKeep); err != nil {
			t.Fatalf("writeFileVersion %d: %v", i, err)
		}
	}
	versions := listFileVersions(dir)
	if len(versions) != fileVersionKeep {
		t.Fatalf("kept %d versions; want %d", len(versions), fileVersionKeep)
	}
	newest, err := os.ReadFile(filepath.Join(dir, versions[0].ID+fileVersionExt))
	if err != nil || string(newest) != "8" {
		t.Fatalf("newest version = %q, %v; want \"8\"", newest, err)
	}
	oldest, err := os.ReadFile(filepath.Join(dir, versions[len(versions)-1].ID+fileVersionExt))
	if err != nil || string(oldest) != "4" {
		t.Fatalf("oldest kept version = %q, %v; want \"4\"", oldest, err)
	}
}

func TestListFileVersionsIgnoresStrangers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "abc.bak"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := listFileVersions(dir); len(got) != 0 {
		t.Fatalf("listed %v; want no versions", got)
	}
	if got := listFileVersions(filepath.Join(dir, "missing")); got == nil || len(got) != 0 {
		t.Fatalf("missing dir returned %v; want empty non-nil list", got)
	}
}

func TestSaveFileVersionBeforeWriteSkipsIdenticalAndNewFiles(t *testing.T) {
	base := t.TempDir()
	m := &Manager{baseDir: base}
	file := filepath.Join(base, "server.properties")

	m.saveFileVersionBeforeWrite("s1", "server.properties", file, []byte("a"))
	if got := listFileVersions(m.fileVersionsDir("s1", "server.properties")); len(got) != 0 {
		t.Fatalf("a new file must not create a version, got %v", got)
	}

	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	m.saveFileVersionBeforeWrite("s1", "server.properties", file, []byte("old"))
	if got := listFileVersions(m.fileVersionsDir("s1", "server.properties")); len(got) != 0 {
		t.Fatalf("identical content must not create a version, got %v", got)
	}

	m.saveFileVersionBeforeWrite("s1", "server.properties", file, []byte("new"))
	got := listFileVersions(m.fileVersionsDir("s1", "server.properties"))
	if len(got) != 1 || got[0].Size != 3 {
		t.Fatalf("changed content should keep the old version, got %v", got)
	}
}
