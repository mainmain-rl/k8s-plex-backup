package targz

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type archiveEntry struct {
	content  string
	typeflag byte
	linkname string
}

// Lit une archive .tar.gz et retourne son contenu sous forme de map.
func readTarGzArchive(t *testing.T, archivePath string) map[string]archiveEntry {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("impossible d'ouvrir l'archive %s: %v", archivePath, err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("impossible de créer le lecteur gzip: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	results := make(map[string]archiveEntry)

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("erreur de lecture du header tar: %v", err)
		}

		var contentStr string
		if hdr.Typeflag == tar.TypeReg {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tarReader); err != nil {
				t.Fatalf("erreur de lecture du contenu pour %s: %v", hdr.Name, err)
			}
			contentStr = buf.String()
		}

		results[hdr.Name] = archiveEntry{
			content:  contentStr,
			typeflag: hdr.Typeflag,
			linkname: hdr.Linkname,
		}
	}

	return results
}

func TestTarGzDirectory_BasicAndExclusions(t *testing.T) {
	tempDir := t.TempDir()

	srcDir := filepath.Join(tempDir, "mydata")
	mustMkdir(t, filepath.Join(srcDir, "sub"))
	mustMkdir(t, filepath.Join(srcDir, "sub", "Cache"))

	mustWriteFile(t, filepath.Join(srcDir, "root.txt"), "hello root")
	mustWriteFile(t, filepath.Join(srcDir, "sub", "child.txt"), "hello child")
	mustWriteFile(t, filepath.Join(srcDir, "sub", "Cache", "cached.tmp"), "cache content")
	mustWriteFile(t, filepath.Join(srcDir, "skip.me"), "skip this file")

	destArchive := filepath.Join(tempDir, "output.tar.gz")

	skipped, err := TarGzDirectory(srcDir, destArchive, "Cache", "skip.me")
	if err != nil {
		t.Fatalf("TarGzDirectory a échoué de manière inattendue: %v", err)
	}

	if len(skipped) != 0 {
		t.Errorf("0 chemins ignorés attendus, obtenu: %v", skipped)
	}

	entries := readTarGzArchive(t, destArchive)
	rootDirName := filepath.Base(srcDir)

	// Fichiers devant être inclus
	expectFile(t, entries, rootDirName+"/root.txt", "hello root")
	expectFile(t, entries, rootDirName+"/sub/child.txt", "hello child")

	// Éléments exclus devant être absents
	expectMissing(t, entries, rootDirName+"/skip.me")
	expectMissing(t, entries, rootDirName+"/sub/Cache")
	expectMissing(t, entries, rootDirName+"/sub/Cache/cached.tmp")
}

func TestTarGzDirectory_RootNotExcluded(t *testing.T) {
	tempDir := t.TempDir()

	// Nom de dossier racine identique au nom exclu
	srcDir := filepath.Join(tempDir, "Cache")
	mustMkdir(t, srcDir)
	mustWriteFile(t, filepath.Join(srcDir, "file.txt"), "data")

	destArchive := filepath.Join(tempDir, "output.tar.gz")

	skipped, err := TarGzDirectory(srcDir, destArchive, "Cache")
	if err != nil {
		t.Fatalf("TarGzDirectory a échoué: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("0 chemins ignorés attendus, obtenu: %v", skipped)
	}

	entries := readTarGzArchive(t, destArchive)
	expectFile(t, entries, "Cache/file.txt", "data")
}

func TestTarGzDirectory_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Test des liens symboliques ignoré sous Windows")
	}

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "symdata")
	mustMkdir(t, srcDir)

	targetFile := filepath.Join(srcDir, "target.txt")
	mustWriteFile(t, targetFile, "target content")

	symlinkPath := filepath.Join(srcDir, "link.txt")
	if err := os.Symlink("target.txt", symlinkPath); err != nil {
		t.Fatalf("erreur création symlink: %v", err)
	}

	destArchive := filepath.Join(tempDir, "output.tar.gz")
	_, err := TarGzDirectory(srcDir, destArchive)
	if err != nil {
		t.Fatalf("TarGzDirectory a échoué: %v", err)
	}

	entries := readTarGzArchive(t, destArchive)
	rootDirName := filepath.Base(srcDir)

	symEntry, ok := entries[rootDirName+"/link.txt"]
	if !ok {
		t.Fatalf("lien symbolique %s/link.txt absent de l'archive", rootDirName)
	}

	if symEntry.typeflag != tar.TypeSymlink {
		t.Errorf("type attendu Symlink (%c), obtenu: %c", tar.TypeSymlink, symEntry.typeflag)
	}
	if symEntry.linkname != "target.txt" {
		t.Errorf("cible du lien attendue 'target.txt', obtenue: '%s'", symEntry.linkname)
	}
}

func TestTarGzDirectory_PermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Test de permissions ignoré lors d'une exécution en root")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Test de permissions ignoré sous Windows")
	}

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "permdata")
	mustMkdir(t, srcDir)

	unreadableFile := filepath.Join(srcDir, "secret.txt")
	mustWriteFile(t, unreadableFile, "secret")
	if err := os.Chmod(unreadableFile, 0o000); err != nil {
		t.Fatalf("impossible de retirer les permissions: %v", err)
	}
	defer os.Chmod(unreadableFile, 0o644)

	destArchive := filepath.Join(tempDir, "output.tar.gz")
	skipped, err := TarGzDirectory(srcDir, destArchive)
	if err != nil {
		t.Fatalf("TarGzDirectory ne doit pas planter sur un refus de permission, erreur: %v", err)
	}

	if len(skipped) != 1 || skipped[0] != unreadableFile {
		t.Errorf("skippedPaths devrait contenir %s, obtenu: %v", unreadableFile, skipped)
	}
}

func TestTarGzDirectory_InvalidDestFile(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	mustMkdir(t, srcDir)

	invalidDest := filepath.Join(tempDir, "dossier_inexistant", "out.tar.gz")
	_, err := TarGzDirectory(srcDir, invalidDest)
	if err == nil {
		t.Error("une erreur était attendue avec un dossier de destination invalide")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("erreur création dossier %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("erreur écriture fichier %s: %v", path, err)
	}
}

func expectFile(t *testing.T, entries map[string]archiveEntry, name, expectedContent string) {
	t.Helper()
	entry, ok := entries[name]
	if !ok {
		t.Errorf("l'élément %s devrait exister dans l'archive", name)
		return
	}
	if entry.typeflag != tar.TypeReg {
		t.Errorf("l'élément %s n'est pas un fichier régulier", name)
	}
	if entry.content != expectedContent {
		t.Errorf("contenu pour %s incorrect: attendu %q, obtenu %q", name, expectedContent, entry.content)
	}
}

func expectMissing(t *testing.T, entries map[string]archiveEntry, name string) {
	t.Helper()
	if _, ok := entries[name]; ok {
		t.Errorf("l'élément %s devrait être absent de l'archive", name)
	}
}
