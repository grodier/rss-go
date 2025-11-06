package ui

import (
	"embed"
	"io/fs"
	"path/filepath"
)

//go:embed "html" "static"
var Files embed.FS

type noDirEmbedFS struct {
	fs embed.FS
}

var NoDirFiles = noDirEmbedFS{fs: Files}

func (n noDirEmbedFS) Open(name string) (fs.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		index := filepath.Join(name, "index.html")
		if _, err := n.fs.Open(index); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return nil, closeErr
			}

			return nil, err
		}
	}
	return f, nil
}
