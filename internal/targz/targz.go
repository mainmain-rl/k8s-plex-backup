package targz

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// TarGzDirectory create an archive .tar.gz (destFile) of the directory
// srcDir. It will include all files and subdirectories, preserving the directory structure.
// srcDir: The source directory to be archived.
// destFile: The destination .tar.gz file path.
func TarGzDirectory(srcDir, destFile string) (err error) {
	out, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("error creating file %s: %w", destFile, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("error closing file %s: %w", destFile, cerr)
		}
	}()

	gzWriter := gzip.NewWriter(out)
	defer func() {
		if cerr := gzWriter.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("error closing gzip writer: %w", cerr)
		}
	}()

	tarWriter := tar.NewWriter(gzWriter)
	defer func() {
		if cerr := tarWriter.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("error closing tar writer: %w", cerr)
		}
	}()

	srcDir = filepath.Clean(srcDir)

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, relErr := filepath.Rel(filepath.Dir(srcDir), path)
		if relErr != nil {
			return relErr
		}

		var link string
		isSymlink := info.Mode()&os.ModeSymlink != 0
		if isSymlink {
			link, relErr = os.Readlink(path)
			if relErr != nil {
				return fmt.Errorf("error reading symlink %s: %w", path, relErr)
			}
		}

		header, hErr := tar.FileInfoHeader(info, link)
		if hErr != nil {
			return fmt.Errorf("error creating tar header for %s: %w", path, hErr)
		}
		header.Name = filepath.ToSlash(relPath)

		if wErr := tarWriter.WriteHeader(header); wErr != nil {
			return fmt.Errorf("error writing header for %s: %w", path, wErr)
		}

		// Nothing to copy for a directory or a symbolic link.
		if info.IsDir() || isSymlink {
			return nil
		}

		f, oErr := os.Open(path)
		if oErr != nil {
			return fmt.Errorf("error opening %s: %w", path, oErr)
		}
		defer f.Close()

		if _, cErr := io.Copy(tarWriter, f); cErr != nil {
			return fmt.Errorf("error copying content of %s: %w", path, cErr)
		}

		return nil
	})

	return err
}
