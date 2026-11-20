package research

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// HashTree hashes every regular file under dir by relative path, exec
// bit and bytes, in lexical walk order. Anything but a regular file or
// a directory is an error, so a symlink cannot hide a change.
func HashTree(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("research: hash: %s is not a regular file", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		exec := byte('0')
		if fi.Mode().Perm()&0o111 != 0 {
			exec = '1'
		}
		binary.Write(h, binary.BigEndian, uint64(len(rel)))
		io.WriteString(h, filepath.ToSlash(rel))
		h.Write([]byte{exec})
		binary.Write(h, binary.BigEndian, uint64(fi.Size()))
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		return err
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
