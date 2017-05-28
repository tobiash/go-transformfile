package transformfile

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
)

/*
Internal data-structure for transformed files
Allow block-wise transformation of a file while preserving file-like
(random access) capabilities.
*/
type rws struct {
	blockSize     int64
	blockOverhead int
	index         int64
	io.Reader
	io.Writer
	io.Seeker
	currentBlock    []byte
	currentBlockIdx int64
	atEOF           bool
}

var (
	/* ErrInvalidSeek marks invalid seek operations	*/
	ErrInvalidSeek         = fmt.Errorf("invalid argument")
	errUnsupportedSeekMode = fmt.Errorf("unsupported seek mode")
)

/*
NewReadWriteSeeker takes transforming readers and writers and wraps around a seeker

	blockSize 			The size of a block of data
	blockOverhead 		The amount of overhead in a block.
						blockSize + blockOverhead is the actual space used
*/
func NewReadWriteSeeker(
	blockSize int64,
	blockOverhead int,
	seeker io.Seeker,
	reader io.Reader,
	writer io.Writer,
) io.ReadWriteSeeker {
	return &rws{blockSize, blockOverhead, 0, reader, writer, seeker, nil, -1, false}
}

func (f *rws) Seek(offset int64, whence int) (int64, error) {
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
		return f.index, errUnsupportedSeekMode
	}
}

func (f *rws) Write(p []byte) (n int, err error) {
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
		b, copied := mergeBlocks(f.currentBlock, p[n:], blockOffset, f.blockSize)
		n += copied
		f.index += int64(copied)
		f.currentBlock = b
		err = f.flushCurrentBlock()
		if err != nil {
			return n, errors.Wrap(err, "Error flushing block")
		}
	}
	return n, nil
}

func (f *rws) Read(p []byte) (n int, err error) {
	for len(p)-n > 0 && err == nil {
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

func (f *rws) resetCurrentBlock() {
	f.currentBlock = nil
	f.currentBlockIdx = -1
}

func (f *rws) flushCurrentBlock() error {
	if f.currentBlock == nil || f.currentBlockIdx < 0 {
		return nil // Nothing to flush is not an error :-)
	}
	f.Seeker.Seek((f.blockSize+int64(f.blockOverhead))*f.currentBlockIdx, io.SeekStart)
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
func (f *rws) loadBlock() error {
	blockIdx, _ := f.position()
	err := f.seekSourceToBlock(blockIdx)
	if err != nil {
		return errors.Wrap(err, "Error seeking to start of block")
	}
	var b = make([]byte, f.blockSize)
	var n int
	for int64(n) < f.blockSize && err == nil {
		var nn int
		nn, err = f.Reader.Read(b[n:])
		n += nn
	}
	f.currentBlock = b[:n]
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
func (f *rws) seekSourceToBlock(blockIdx int64) error {
	seekTarget := blockIdx * (f.blockSize + int64(f.blockOverhead))
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
func (f *rws) position() (block, offset int64) {
	return f.index / f.blockSize, f.index % f.blockSize
}

// Accounts for block overhead for the given offset
func (f *rws) addOverhead(offset int64) int64 {
	numBlocks := offset / f.blockSize
	if offset%f.blockSize > 0 {
		numBlocks++
	}
	return offset + numBlocks*int64(f.blockOverhead)
}

func (f *rws) removeOverhead(offset int64) int64 {
	bs := f.blockSize + int64(f.blockOverhead)
	numBlocks := offset / bs
	// Probably there is a better way to ceil this? Floats?
	if offset%bs > 0 {
		numBlocks++
	}
	return offset - numBlocks*int64(f.blockOverhead)
}

// Merge insert into block at offset, appending if necessary up to blockSize
// Returns the updated block and the number of bytes inserted
func mergeBlocks(block, insert []byte, offset, blockSize int64) ([]byte, int) {
	slen := int64(len(block))
	if int64(len(block)) < offset {
		block = append(block, make([]byte, offset-int64(len(block)))...)
	}
	n := int(min(int64(len(insert)), blockSize-offset)) // Safe since slice length is within int range
	block = append(block[:offset], insert[:n]...)
	return block[:min(max(slen, int64(len(block))), blockSize)], n
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
