package targz

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// TarGzDirectory creates an archive .tar.gz (destFile) of the directory
// srcDir. It will include all files and subdirectories, preserving the
// directory structure, except any entry listed in excludeNames: each name
// is matched against the base name of every file or directory encountered
// (e.g. "Cache" matches ".../Plex Media Server/Cache" no matter how deep
// it is). A matching directory is skipped along with everything
// underneath it, mirroring `tar --exclude=NAME`. A matching file is
// skipped on its own.
//
// Files, symlinks or directories that can't be read due to a permission
// error are logged and skipped rather than aborting the whole archive
// (e.g. Plex's .LocalAdminToken, only readable by its owning process) —
// their paths are returned in skippedPaths so the caller can decide
// whether that's acceptable. Excluded entries are logged too but are not
// added to skippedPaths, since they were left out on purpose rather than
// because of an error.
func TarGzDirectory(srcDir, destFile string, excludeNames ...string) (skippedPaths []string, err error) {
	out, err := os.Create(destFile)
	if err != nil {
		return nil, fmt.Errorf("error creating file %s: %w", destFile, err)
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

	exclude := make(map[string]struct{}, len(excludeNames))
	for _, name := range excludeNames {
		exclude[name] = struct{}{}
	}

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsPermission(walkErr) {
				log.Printf("skipping %s: permission denied", path)
				skippedPaths = append(skippedPaths, path)
				return nil
			}
			return walkErr
		}

		// Never exclude the root itself, only entries underneath it.
		if path != srcDir {
			if _, excluded := exclude[info.Name()]; excluded {
				if info.IsDir() {
					log.Printf("skipping excluded directory %s", path)
					return filepath.SkipDir
				}
				log.Printf("skipping excluded file %s", path)
				return nil
			}
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
				if os.IsPermission(relErr) {
					log.Printf("skipping %s: permission denied", path)
					skippedPaths = append(skippedPaths, path)
					return nil
				}
				return fmt.Errorf("error reading symlink %s: %w", path, relErr)
			}
		}

		var f *os.File
		if !info.IsDir() && !isSymlink {
			var oErr error
			f, oErr = os.Open(path)
			if oErr != nil {
				if os.IsPermission(oErr) {
					log.Printf("skipping %s: permission denied", path)
					skippedPaths = append(skippedPaths, path)
					return nil
				}
				return fmt.Errorf("error opening %s: %w", path, oErr)
			}
			defer f.Close()
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

		if _, cErr := io.Copy(tarWriter, f); cErr != nil {
			return fmt.Errorf("error copying content of %s: %w", path, cErr)
		}

		return nil
	})

	return skippedPaths, err
}
