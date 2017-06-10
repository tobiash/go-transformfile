package naclfs

import (
	"github.com/spf13/afero"
	"github.com/tobiash/go-transformfile/naclfs/nacltr"
	"github.com/tobiash/go-transformfile/trfs"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/text/transform"
)

const FS_NAME = "naclfs"

func New(blockSize int64, key *[32]byte, backing afero.Fs) afero.Fs {

	readTr := func() transform.Transformer {
		return nacltr.NewDecryptTransformer(key, blockSize)
	}
	writeTr := func() transform.Transformer {
		return nacltr.NewEncryptTransformer(key, blockSize)
	}

	return trfs.NewTransformFileFs(
		blockSize,
		nacltr.NONCE_SIZE+secretbox.Overhead,
		FS_NAME,
		backing,
		readTr, writeTr,
	)
}
