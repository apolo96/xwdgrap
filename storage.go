package gitfresh

import (
	"log/slog"
	"os"
	"path/filepath"
)

type FlatFiler interface {
	Write(data []byte) (n int, err error)
	Read() (n []byte, err error)
}

type FlatFile struct {
	Name string
	Path string
}

func (f *FlatFile) Write(data []byte) (n int, err error) {
	if err := os.MkdirAll(f.Path, os.ModePerm); err != nil {
		slog.Error(err.Error())
		return 0, err
	}
	file := filepath.Join(f.Path, f.Name)
	err = os.WriteFile(file, data, 0644)
	if err != nil {
		slog.Error(err.Error())
		return 0, err
	}
	slog.Info("config file created successfully", "path", f.Path, "file", f.Name)
	return len(data), nil
}

func (f *FlatFile) Read() (n []byte, err error) {
	path := filepath.Join(f.Path, f.Name)
	file, err := os.ReadFile(path)
	if err != nil {
		slog.Error(err.Error())
		return []byte{}, err
	}
	return file, nil
}
