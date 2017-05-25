package transformfile

import (
	"fmt"
	"io"
	"os"
	"strings"
)

/*
File interface, compatible with normal files
*/
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	io.WriterAt
	Name() string
	Readdir(count int) ([]os.FileInfo, error)
	Readdirnames(n int) ([]string, error)
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(size int64) error
	WriteString(s string) (int, error)
}

type file struct {
	rws
	backing File
}

/*
New creates a file wrapper around a backing file, using a transforming
reader and writer
*/
func New(
	blockSize int64,
	blockOverhead int64,
	backing File,
	reader io.Reader,
	writer io.Writer,
) File {
	return &file{
		rws{blockSize, blockOverhead, 0, reader, writer, backing, nil, -1, false},
		backing,
	}
}

func (f *file) Name() string {
	return f.backing.Name()
}

func (f *file) Close() error {
	syncErr := f.Sync()
	closeErr := f.backing.Close()
	return combineErrors(syncErr, closeErr)
}

func (f *file) ReadAt(p []byte, off int64) (int, error) {
	_, err := f.rws.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}
	return f.rws.Read(p)
}

func (f *file) WriteAt(p []byte, off int64) (int, error) {
	_, err := f.rws.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}
	return f.rws.Write(p)
}

func (f *file) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s))
}

func (f *file) Readdir(count int) ([]os.FileInfo, error) {
	return f.backing.Readdir(count)
}

func (f *file) Readdirnames(n int) ([]string, error) {
	return f.backing.Readdirnames(n)
}

func (f *file) Stat() (os.FileInfo, error) {
	// TODO Account for overhead
	return f.backing.Stat()
}

func (f *file) Sync() error {
	flushErr := f.rws.flushCurrentBlock()
	syncErr := f.backing.Sync()
	return combineErrors(flushErr, syncErr)
}

func (f *file) Truncate(size int64) error {
	// Calculate size to take overhead into account
	// Rewrite last block
	return fmt.Errorf("Truncating not implemented yet")
}

func combineErrors(errs ...error) error {
	var errstrings []string
	for _, e := range errs {
		if e != nil {
			errstrings = append(errstrings, e.Error())
		}
	}
	if len(errstrings) > 0 {
		return fmt.Errorf(strings.Join(errstrings, "\n"))
	}
	return nil
}
