package trfs

import (
	"io"
	"os"

	"github.com/spf13/afero"
	"github.com/tobiash/go-transformfile"
	"golang.org/x/text/transform"
)

type trfs struct {
	afero.Fs
	name                   string
	blockSize              int64
	overhead               int
	createReadTransformer  func() transform.Transformer
	createWriteTransformer func() transform.Transformer
}

/*
NewTransformFileFs creates a new filesystem that passes files through the given transformations.
File stats accounts for transform overhead, but filenames are not changed.
*/
func NewTransformFileFs(
	blockSize int64,
	overhead int,
	name string,
	backing afero.Fs,
	readTr, writeTr func() transform.Transformer) afero.Fs {
	return &trfs{backing, name, blockSize, overhead, readTr, writeTr}
}

func (fs *trfs) newFile(f afero.File, readOnly bool) afero.File {
	readTr := fs.createReadTransformer()
	writeTr := fs.createWriteTransformer()
	return transformfile.NewFromTransformer(
		fs.blockSize,
		fs.overhead,
		f,
		readOnly,
		readTr,
		writeTr,
	)
}

func (fs *trfs) Create(name string) (afero.File, error) {
	f, err := fs.Fs.Create(name)
	if err != nil {
		return nil, err
	}
	return fs.newFile(f, false), nil
}

func (fs *trfs) Open(name string) (afero.File, error) {
	f, err := fs.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return fs.newFile(f, false), nil
}

func (fs *trfs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	// WR_ONLY can not be passed down
	if flag&os.O_WRONLY != 0 {
		flag &= ^os.O_WRONLY
		flag |= os.O_RDWR
	}
	f, err := fs.Fs.OpenFile(name, flag&^os.O_APPEND, perm)
	if err != nil {
		return nil, err
	}
	readOnly := flag&os.O_RDONLY != 0
	n := fs.newFile(f, readOnly)
	if flag&os.O_APPEND > 0 && n != nil {
		n.Seek(0, io.SeekEnd)
	}
	return n, nil
}

func (fs *trfs) Stat(name string) (os.FileInfo, error) {
	// TODO Account for overhead in file sizes?
	return fs.Fs.Stat(name)
}

func (fs *trfs) Name() string {
	return fs.name
}
