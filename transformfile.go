package transformfile

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

/*
Allow block-wise transformation of a file while preserving file-like
(random access) capabilities.
*/
type file struct {
	blockSize     int64
	blockOverhead int64
	index         int64
	io.Reader
	io.Writer
	io.Seeker
	currentBlock    []byte
	currentBlockIdx int64
	atEOF           bool
}

var (
	ErrInvalidSeek         = fmt.Errorf("invalid argument")
	ErrUnsupportedSeekMode = fmt.Errorf("unsupported seek mode")
)

/*
New transform file

	blockSize 			The size of the block in the underlying file
	blockOverhead 		The amount of overhead in a block.
						blockSize - blockOverhead is the actual space for data
*/
func New(blockSize int64, blockOverhead int64, seeker io.Seeker, reader io.Reader, writer io.Writer) io.ReadWriteSeeker {
	return &file{blockSize, blockOverhead, 0, reader, writer, seeker, nil, -1, false}
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		sPos := f.addOverhead(offset)
		if sPos < 0 {
			return 0, ErrInvalidSeek
		}
		nIdx, err := f.Seeker.Seek(sPos, io.SeekStart)
		f.index = f.removeOverhead(nIdx)
		return f.index, err
	case io.SeekEnd:
		endOffset, err := f.Seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return f.index, err
		}
		return f.Seek(f.removeOverhead(endOffset)+offset, io.SeekStart)
	case io.SeekCurrent:
		return f.Seek(f.index+offset, io.SeekStart)
	default:
		return f.index, ErrUnsupportedSeekMode
	}
}

func (f *file) Write(p []byte) (n int, err error) {
	for len(p)-n > 0 {
		err = f.loadBlock()
		if err != nil {
			return n, errors.Wrap(err, "Error reading next block")
		}
		_, blockOffset := f.position()
		if blockOffset < 0 || blockOffset > int64(len(f.currentBlock)) {
			// TODO
			return n, fmt.Errorf("Invalid offset %d", blockOffset)
		}
		copied := copy(f.currentBlock[blockOffset:], p[n:])
		n += copied
		f.index += int64(copied)
		err = f.flushCurrentBlock()
		if err != nil {
			return n, errors.Wrap(err, "Error flushing block")
		}
	}
	return n, nil
}

func (f *file) Read(p []byte) (n int, err error) {
	for len(p)-n > 0 {
		err = f.loadBlock()
		_, blockOffset := f.position()
		if blockOffset < 0 || blockOffset > int64(len(f.currentBlock)) {
			return n, ErrInvalidSeek
		}
		copied := copy(p[n:], f.currentBlock[blockOffset:])
		n += copied
		f.index += int64(copied)
		if f.atEOF && n < len(p) {
			return n, io.EOF
		}
	}
	return n, err
}

func (f *file) resetCurrentBlock() {
	f.currentBlock = nil
	f.currentBlockIdx = -1
}

func (f *file) flushCurrentBlock() error {
	if f.currentBlock == nil || f.currentBlockIdx < 0 {
		return nil // Nothing to flush is not an error :-)
	}
	f.Seeker.Seek(f.blockSize*f.currentBlockIdx, io.SeekStart)
	written, err := f.Writer.Write(f.currentBlock)
	if err != nil {
		return err
	}
	if written != len(f.currentBlock) {
		return fmt.Errorf("Could write block, %d bytes written, block size was %d", written, len(f.currentBlock))
	}
	return nil
}

// Loads the block for the current index
func (f *file) loadBlock() error {
	blockIdx, _ := f.position()
	err := f.seekSourceToBlock(blockIdx)
	if err != nil {
		return errors.Wrap(err, "Error seeking to start of block")
	}
	var b = make([]byte, f.blockSize-f.blockOverhead)
	read, err := f.Reader.Read(b)
	f.currentBlock = b[:read]
	f.currentBlockIdx = blockIdx

	if err == io.EOF {
		f.atEOF = true
	} else {
		f.atEOF = false
		if err != nil {
			return errors.Wrap(err, "Error reading block")
		}
	}
	return nil
}

// Seeks the source file to the start of the given block
func (f *file) seekSourceToBlock(blockIdx int64) error {
	seekTarget := blockIdx * f.blockSize
	if seekTarget < 0 {
		return ErrInvalidSeek
	}
	seekResult, err := f.Seeker.Seek(seekTarget, io.SeekStart)
	if err != nil {
		return errors.Wrap(err, "Seek error")
	}
	if seekResult != seekTarget {
		return fmt.Errorf("Unexpected seek result, seeking for %d resulted in offset %d", seekTarget, seekResult)
	}
	return nil
}

// Returns the block that contains the current index
// as well as the offset of the position within the block
func (f *file) position() (block, offset int64) {
	return f.index / (f.blockSize - f.blockOverhead), f.index % (f.blockSize - f.blockOverhead)
}

// Accounts for block overhead for the given offset
func (f *file) addOverhead(offset int64) int64 {
	numBlocks := offset / (f.blockSize - f.blockOverhead)
	return offset + numBlocks*f.blockOverhead
}

func (f *file) removeOverhead(offset int64) int64 {
	numBlocks := offset / f.blockSize
	return offset - numBlocks*f.blockOverhead
}

// func (f *file) Name() string {
// 	return f.backingFile.Name()
// }

// func (f *file) Readdir(count int) ([]os.FileInfo, error) {
// 	return f.backingFile.Readdir(count)
// }

// func (f *file) Readdirnames(n int) ([]string, error) {
// 	return f.backingFile.Readdirnames(n)
// }

// func (f *file) Stat() (os.FileInfo, error) {
// 	return f.backingFile.Stat()
// }

// func (f *file) Sync() error {
// 	return f.backingFile.Sync()
// }

// func (f *file) Truncate(size int64) error {
// 	// Calculate size to take overhead into account

// 	return f.backingFile.Truncate(size)
// }

// func (f *file) WriteString(s string) (ret int, err error) {
// 	return f.Write([]byte(s))
// }

// func (f *file) Write(p []byte) (n int, err error) {
// 	return f.Write(p)
// }

// func (f *file) Read(p []byte) (n int, err error) {
// 	return f.Read(p)
// }

// func (f *file) Close() error {
// 	return f.backingFile.Close()
// }

// func (f *file) Seek(offset int64, whence int) (int64, error) {
// 	return f.backingFile.Seek(offset, whence)
// }

// func (f *file) ReadAt(p []byte, off int64) (int, error) {
// 	return f.backingFile.ReadAt(p, off)
// }

// func (f *file) WriteAt(p []byte, off int64) (int, error) {
// 	return f.backingFile.WriteAt(p, off)
// }
