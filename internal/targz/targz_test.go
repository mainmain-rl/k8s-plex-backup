package targz

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// readArchive lit une archive .tar.gz et retourne une map "chemin dans
// l'archive" -> contenu. Pour un répertoire, le contenu vaut nil. Pour un
// lien symbolique, le contenu est "symlink:<cible>".
func readArchive(t *testing.T, archivePath string) map[string][]byte {
	t.Helper()

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("impossible d'ouvrir l'archive: %v", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("flux gzip invalide: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	entries := make(map[string][]byte)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("erreur de lecture du tar: %v", err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			entries[header.Name] = nil
		case tar.TypeReg:
			content, err := io.ReadAll(tarReader)
			if err != nil {
				t.Fatalf("erreur de lecture du contenu de %s: %v", header.Name, err)
			}
			entries[header.Name] = content
		case tar.TypeSymlink:
			entries[header.Name] = []byte("symlink:" + header.Linkname)
		}
	}

	return entries
}

func TestTarGzDirectory(t *testing.T) {
	srcDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("world"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	destFile := filepath.Join(t.TempDir(), "archive.tar.gz")

	skipped, err := TarGzDirectory(srcDir, destFile)
	if err != nil {
		t.Fatalf("TarGzDirectory() error = %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("aucun fichier ne devrait être sauté ici, obtenu %v", skipped)
	}
	if _, err := os.Stat(destFile); err != nil {
		t.Fatalf("l'archive n'a pas été créée: %v", err)
	}

	entries := readArchive(t, destFile)
	rootName := filepath.Base(srcDir)

	wantFile1 := rootName + "/file1.txt"
	if content, ok := entries[wantFile1]; !ok {
		t.Errorf("entrée manquante: %s", wantFile1)
	} else if string(content) != "hello" {
		t.Errorf("contenu de %s = %q, attendu %q", wantFile1, content, "hello")
	}

	wantSubdir := rootName + "/subdir"
	if _, ok := entries[wantSubdir]; !ok {
		t.Errorf("entrée de répertoire manquante: %s", wantSubdir)
	}

	wantFile2 := rootName + "/subdir/file2.txt"
	if content, ok := entries[wantFile2]; !ok {
		t.Errorf("entrée manquante: %s", wantFile2)
	} else if string(content) != "world" {
		t.Errorf("contenu de %s = %q, attendu %q", wantFile2, content, "world")
	}
}

func TestTarGzDirectory_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("création de lien symbolique nécessite des privilèges sur Windows")
	}

	srcDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "real.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(srcDir, "link.txt")); err != nil {
		t.Fatalf("setup symlink: %v", err)
	}

	destFile := filepath.Join(t.TempDir(), "archive.tar.gz")
	skipped, err := TarGzDirectory(srcDir, destFile)
	if err != nil {
		t.Fatalf("TarGzDirectory() error = %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("aucun fichier ne devrait être sauté ici, obtenu %v", skipped)
	}

	entries := readArchive(t, destFile)
	rootName := filepath.Base(srcDir)
	wantLink := rootName + "/link.txt"

	content, ok := entries[wantLink]
	if !ok {
		t.Fatalf("entrée manquante pour le lien symbolique: %s", wantLink)
	}
	if string(content) != "symlink:real.txt" {
		t.Errorf("lien = %q, attendu %q", content, "symlink:real.txt")
	}
}

func TestTarGzDirectory_EmptyDirectory(t *testing.T) {
	srcDir := t.TempDir()
	destFile := filepath.Join(t.TempDir(), "archive.tar.gz")

	skipped, err := TarGzDirectory(srcDir, destFile)
	if err != nil {
		t.Fatalf("TarGzDirectory() error = %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("aucun fichier ne devrait être sauté ici, obtenu %v", skipped)
	}

	entries := readArchive(t, destFile)
	rootName := filepath.Base(srcDir)

	if _, ok := entries[rootName]; !ok {
		t.Errorf("entrée de répertoire racine manquante: %s", rootName)
	}
	if len(entries) != 1 {
		t.Errorf("attendu 1 entrée (le répertoire racine), obtenu %d: %v", len(entries), entries)
	}
}

func TestTarGzDirectory_SourceDoesNotExist(t *testing.T) {
	destFile := filepath.Join(t.TempDir(), "archive.tar.gz")
	missingSrc := filepath.Join(t.TempDir(), "does-not-exist")

	if _, err := TarGzDirectory(missingSrc, destFile); err == nil {
		t.Fatal("erreur attendue pour un répertoire source inexistant, obtenu nil")
	}
}

func TestTarGzDirectory_InvalidDestination(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Répertoire parent inexistant -> os.Create doit échouer.
	destFile := filepath.Join(srcDir, "nope", "archive.tar.gz")

	if _, err := TarGzDirectory(srcDir, destFile); err == nil {
		t.Fatal("erreur attendue pour une destination invalide, obtenu nil")
	}
}

func TestTarGzDirectory_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("les bits de permission Unix ne s'appliquent pas de la même façon sous Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("test invalide en root: les permissions de fichier sont ignorées")
	}

	srcDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(srcDir, "readable.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	restrictedPath := filepath.Join(srcDir, "restricted.txt")
	if err := os.WriteFile(restrictedPath, []byte("secret"), 0o000); err != nil {
		t.Fatalf("setup: %v", err)
	}

	destFile := filepath.Join(t.TempDir(), "archive.tar.gz")

	skipped, err := TarGzDirectory(srcDir, destFile)
	if err != nil {
		t.Fatalf("TarGzDirectory() error = %v, attendu nil (le fichier restreint doit juste être ignoré)", err)
	}
	if len(skipped) != 1 || skipped[0] != restrictedPath {
		t.Errorf("skipped = %v, attendu [%s]", skipped, restrictedPath)
	}

	entries := readArchive(t, destFile)
	rootName := filepath.Base(srcDir)

	if _, ok := entries[rootName+"/readable.txt"]; !ok {
		t.Errorf("le fichier lisible aurait dû être présent dans l'archive")
	}
	if _, ok := entries[rootName+"/restricted.txt"]; ok {
		t.Errorf("le fichier sans permission n'aurait pas dû être présent dans l'archive")
	}
}
