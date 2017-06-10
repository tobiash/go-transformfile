package nacltr

import (
	"bytes"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/text/transform"
)

const NONCE_SIZE = 24

var (
	errShortInternal = errors.New("transform: short internal buffer")
	errDecrypt       = errors.New("could not decrypt or authenticate data")
)

type secretboxTransformer struct {
	key       *[32]byte
	blockSize int64
	buffer    *bytes.Buffer
}

type secretboxEncryptTransformer struct {
	*secretboxTransformer
}

type secretboxDecryptTransformer struct {
	*secretboxTransformer
}

func NewEncryptTransformer(key *[32]byte, blockSize int64) transform.Transformer {
	return &secretboxEncryptTransformer{
		&secretboxTransformer{
			key,
			blockSize,
			new(bytes.Buffer),
		},
	}
}

func NewDecryptTransformer(key *[32]byte, blockSize int64) transform.Transformer {
	return &secretboxDecryptTransformer{
		&secretboxTransformer{
			key, blockSize, new(bytes.Buffer),
		},
	}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (s *secretboxEncryptTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	// Append src to buffer
	buffered, err := s.buffer.Write(src)
	if err != nil {
		return 0, buffered, err
	}
	if buffered < len(src) {
		return 0, buffered, errShortInternal
	}
	// Check if len buffer > blockSize
	if int64(s.buffer.Len()) < s.blockSize && !atEOF {
		return 0, buffered, transform.ErrShortSrc
	}
	var expectedLen = NONCE_SIZE + secretbox.Overhead + s.blockSize
	if int64(len(dst)) < expectedLen {
		return 0, buffered, transform.ErrShortDst
	}
	data := make([]byte, min(s.blockSize, int64(s.buffer.Len())))
	_, err = s.buffer.Read(data)

	if err != nil {
		return 0, len(src), err
	}
	s.buffer = new(bytes.Buffer)

	var nonce [NONCE_SIZE]byte
	var res = make([]byte, NONCE_SIZE)
	rand.Read(nonce[:])
	copy(res, nonce[:])
	res = secretbox.Seal(res, data, &nonce, s.key)
	copy(dst, res)
	return len(nonce) + len(data) + secretbox.Overhead, len(src), nil
}

func (s *secretboxDecryptTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	// Append src to buffer
	buffered, err := s.buffer.Write(src)
	if err != nil {
		return 0, buffered, err
	}
	if buffered < len(src) {
		return 0, buffered, errShortInternal
	}
	expectedLen := NONCE_SIZE + secretbox.Overhead + s.blockSize
	if int64(s.buffer.Len()) < expectedLen && !atEOF {
		return 0, len(src), transform.ErrShortSrc
	}
	actualLen := min(s.blockSize+secretbox.Overhead, int64(s.buffer.Len())-NONCE_SIZE)
	if actualLen <= 0 {
		return 0, len(src), nil
	}
	if int64(len(dst)) < s.blockSize {
		return 0, len(src), transform.ErrShortDst
	}
	var ciphertext = make([]byte, actualLen)
	var nonce [NONCE_SIZE]byte
	_, err = s.buffer.Read(nonce[:])
	if err != nil {
		return 0, len(src), err
	}
	_, err = s.buffer.Read(ciphertext)
	s.buffer = new(bytes.Buffer)
	if err != nil {
		return 0, len(src), err
	}
	var res []byte
	res, ok := secretbox.Open(res, ciphertext, &nonce, s.key)
	if ok {
		copy(dst, res)
		return len(res), len(src), nil
	}
	return 0, len(src), errDecrypt
}

func (s *secretboxTransformer) Reset() {
	s.buffer = new(bytes.Buffer)
}
