package transformfile

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"os"

	"github.com/spf13/afero"
)

// TODO Some unused testing utilities

type seekableBuffer struct {
	Buffer *bytes.Buffer
	Index  int64
}

func (sbuf *seekableBuffer) Read(p []byte) (n int, err error) {
	n, err = bytes.NewBuffer(sbuf.Buffer.Bytes()[sbuf.Index:]).Read(p)
	sbuf.Index += int64(n)
	return
}

func (sbuf *seekableBuffer) Seek(offset int64, whence int) (off int64, err error) {
	switch whence {
	case io.SeekStart:
		if offset >= int64(sbuf.Buffer.Len()) || offset < 0 {
			err = fmt.Errorf("Invalid offset %d", offset)
		} else {
			sbuf.Index = offset
			off = sbuf.Index
		}
	case io.SeekEnd:
		// TODO Not sure if Len() or Len()-1
		return sbuf.Seek(int64(sbuf.Buffer.Len())+offset, io.SeekStart)
	case io.SeekCurrent:
		return sbuf.Seek(sbuf.Index+offset, io.SeekStart)
	default:
		err = fmt.Errorf("Unsupported Seek Method: %d", whence)
	}
	return
}

// Wraps a seeker and doubles offsets
type doubleSeeker struct {
	io.Seeker
}

func (ds *doubleSeeker) Seek(offset int64, whence int) (int64, error) {
	nOff, err := ds.Seeker.Seek(offset*2, whence)
	return nOff / 2, err
}

func NewDoubleSeeker(s io.Seeker) io.Seeker {
	return &doubleSeeker{s}
}

// Wraps a seeker and halves offsets
//
type halfSeeker struct {
	io.Seeker
}

func (ds *halfSeeker) Seek(offset int64, whence int) (int64, error) {
	nOff, err := ds.Seeker.Seek(offset/2, whence)
	return nOff * 2, err
}

func NewHalfSeeker(s io.Seeker) io.Seeker {
	return &halfSeeker{s}
}

// Wraps a reader and doubles each byte read
type doubleReader struct {
	io.Reader
}

func (dr *doubleReader) Read(p []byte) (int, error) {
	pHalf := make([]byte, len(p)/2)
	hRead, err := dr.Reader.Read(pHalf)
	var i int
	for i = 0; i < hRead; i++ {
		p[i*2] = pHalf[i]
		p[i*2+1] = pHalf[i]
	}
	return hRead * 2, err
}

func NewDoubleReader(r io.Reader) io.Reader {
	return &doubleReader{r}
}

// Wraps a reader and reads only every second byte
type halfReader struct {
	io.Reader
}

func (hr *halfReader) Read(p []byte) (i int, err error) {
	// We need to read twice the length of the requested slice
	pDouble := make([]byte, len(p)*2)
	dRead, err := hr.Reader.Read(pDouble)
	for i = 0; i*2 < dRead; i++ {
		p[i] = pDouble[i*2]
	}
	return
}

func NewHalfReader(r io.Reader) io.Reader {
	return &halfReader{r}
}

type doubleWriter struct {
	w io.Writer
}

func (dw *doubleWriter) Write(p []byte) (n int, err error) {
	pDouble := make([]byte, len(p)*2)
	for i := 0; i < len(p); i++ {
		pDouble[i*2] = p[i]
		pDouble[i*2+1] = p[i]
	}
	d, err := dw.w.Write(pDouble)
	return d / 2, err
}

func NewDoubleWriter(w io.Writer) io.Writer {
	return &doubleWriter{w}
}

var readerTests = []struct {
	str    string
	double string
	half   string
}{
	{"abcdefgh", "aabbccddeeffgghh", "aceg"},
}

func TestReaders(t *testing.T) {
	for _, tt := range readerTests {
		halfR := NewHalfReader(strings.NewReader(tt.str))
		doubleR := NewDoubleReader(strings.NewReader(tt.str))
		half, err := ioutil.ReadAll(halfR)
		if err != nil {
			t.Errorf("Error reading half %s, %v", tt.str, err)
		}
		if string(half) != tt.half {
			t.Errorf("Unexpected half of \"%s\": Expected \"%s\", got \"%s\"", tt.str, tt.half, string(half))
		}
		double, err := ioutil.ReadAll(doubleR)
		if err != nil {
			t.Errorf("Error reading double %s, %v", tt.str, err)
		}
		if string(double) != tt.double {
			t.Errorf("Unexpected double of \"%s\": Expected \"%s\", got \"%s\"", tt.str, tt.double, string(double))
		}
	}
}

var doubleSeekReaderTests = []struct {
	data     string
	off      int64
	whence   int
	expected string
}{
	{"hello world", 0, io.SeekStart, "hheelloo  wwoorrlldd"},
	{"hello world", 5, io.SeekStart, "wwoorrlldd"},
	// {"ab", 0, "aabb"},
	// {"ab", 1, "abb"},
}

var readTests = []struct {
	startOffset int64
	readLen     int
	expected    []byte
	expectedErr error
}{
	{0, 5, []byte("Hello"), nil},
	{10, 10, []byte("d"), io.EOF},
	{11, 10, []byte(""), io.EOF},
	{0, 20, []byte("Hello World"), io.EOF},
}

func TestReadFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	err := afero.WriteFile(fs, "test", []byte("HHeelllloo  WWoorrlldd"), 755)
	if err != nil {
		t.Error(err)
		return
	}
	for _, tt := range readTests {
		file, err := fs.OpenFile("test", os.O_RDONLY, 755)
		if err != nil {
			t.Error(err)
		}
		tr := NewReadWriteSeeker(1, 1, file, NewHalfReader(file), nil)
		_, err = tr.Seek(tt.startOffset, io.SeekStart)
		if err != nil {
			t.Error(err)
		}
		var b = make([]byte, tt.readLen)
		n, err := tr.Read(b)
		if err != tt.expectedErr {
			t.Errorf("Unexpected err \"%v\", expected \"%v\"", err, tt.expectedErr)
		}
		// if n != tt.readLen {
		// 	t.Errorf("Unexpected count %d, expected %d", n, tt.readLen)
		// }
		if string(b[:n]) != string(tt.expected) {
			t.Errorf("Unexpected result %s, expected %s", string(b[:n]), string(tt.expected))
		}
	}
}

func TestWriteIntoEmptyFile(t *testing.T) {
	fs := afero.NewMemMapFs()
	file, _ := fs.OpenFile("test", os.O_CREATE|os.O_RDWR, 755)
	tr := NewReadWriteSeeker(1, 1, file, NewHalfReader(file), NewDoubleWriter(file))
	data := []byte("Hello, World")
	w, err := tr.Write([]byte(data))
	if err != nil {
		t.Error(err)
	}
	if w != len(data) {
		t.Errorf("Expected %d bytes to be written, but %d were written", len(data), w)
	}
	tr.Seek(0, io.SeekStart)
	contents, _ := ioutil.ReadAll(tr)
	if string(contents) != string(data) {
		t.Errorf("Unexpected file contents \"%s\"", string(contents))
	}
}

//
var writeTests = []struct {
	startOffset      int64
	data             []byte
	resultData       []byte
	expectedSeekErr  error
	expectedWriteErr error
}{
	{0, []byte("Goodbye"), []byte("Goodbyeorld"), nil, nil},
	{-2, []byte("Foo"), []byte("Foolo World"), ErrInvalidSeek, nil},
}

func TestWriteFile(t *testing.T) {

	for _, tt := range writeTests {
		fs := afero.NewMemMapFs()
		err := afero.WriteFile(fs, "test", []byte("HHeelllloo  WWoorrlldd"), 755)
		if err != nil {
			t.Error(err)
			return
		}
		file, err := fs.OpenFile("test", os.O_RDWR, 755)
		if err != nil {
			t.Error(err)
		}
		tr := NewReadWriteSeeker(1, 1, file, NewHalfReader(file), NewDoubleWriter(file))
		_, err = tr.Seek(tt.startOffset, io.SeekStart)
		if err != tt.expectedSeekErr {
			t.Errorf("Unexpected error %v, expected %v", err, tt.expectedSeekErr)
		}
		n, err := tr.Write(tt.data)
		if err != tt.expectedWriteErr {
			t.Error(err)
		}
		if n != len(tt.data) {
			t.Errorf("Expected %d bytes to be written, but wrote %d", len(tt.data), n)
		}
		_, _ = tr.Seek(0, io.SeekStart)
		completeData, err := ioutil.ReadAll(tr)
		if err != nil {
			t.Error(err)
		}
		if string(completeData) != string(tt.resultData) {
			t.Errorf("Unexpected result \"%s\", expected \"%s\"", completeData, tt.resultData)
		}
	}
}

var seekTests = []struct {
	startOffset    int64
	len            int64
	seekOffset     int64
	seekWhence     int
	expectedOffset int64
	expectedErr    interface{}
}{
	{0, 10, 0, io.SeekStart, 0, nil},
	{0, 10, 5, io.SeekStart, 5, nil},
	{5, 10, 2, io.SeekCurrent, 7, nil},
	{5, 10, -2, io.SeekCurrent, 3, nil},
	{5, 10, 0, io.SeekEnd, 10, nil},
	{5, 10, 2, io.SeekEnd, 12, nil}, // afero memfs does not trigger an error
	{0, 10, -2, io.SeekStart, 0, ErrInvalidSeek},
}

func TestSeekFile(t *testing.T) {
	for _, tt := range seekTests {
		fs := afero.NewMemMapFs()
		file, err := fs.OpenFile("test", os.O_CREATE|os.O_RDWR, 755)
		if err != nil {
			t.Error(err)
		}
		for i := int64(0); i < tt.len*2; i++ {
			_, err := file.Write([]byte("x"))
			if err != nil {
				t.Error(err)
				return
			}
		}
		file.Close()
		file, err = fs.OpenFile("test", os.O_RDONLY, 755)
		if err != nil {
			t.Error(err)
		}

		tr := NewReadWriteSeeker(1, 1, file, NewHalfReader(file), nil)
		sOff, err := tr.Seek(tt.startOffset, io.SeekStart)
		if err != nil {
			t.Error(err)
		}
		if sOff != tt.startOffset {
			t.Errorf("Could not seek to start offset %d, got %d", tt.startOffset, sOff)
		}

		newOffset, err := tr.Seek(tt.seekOffset, tt.seekWhence)
		if err != tt.expectedErr {
			t.Errorf("Unexpected seek error \"%v\", expected \"%v\"", err, tt.expectedErr)
		}
		if newOffset != tt.expectedOffset {
			t.Errorf("Unexpected seek result %d, expected %d", newOffset, tt.expectedOffset)
		}
	}
}
