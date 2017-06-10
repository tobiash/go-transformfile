package nacltr

import (
	"bytes"
	"fmt"
	"testing"

	"golang.org/x/text/transform"
)

func TestEncryptTransformer(t *testing.T) {
	var key [32]byte
	copy(key[:], "passcode")
	secret := []byte("secret")
	transformer := NewEncryptTransformer(&key, 32)
	transformed, _, err := transform.Bytes(transformer, secret)
	if err != nil {
		t.Fatal(err)
	}
	decrypter := NewDecryptTransformer(&key, 32)
	decrypted, _, err := transform.Bytes(decrypter, transformed)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Compare(decrypted, secret) != 0 {
		fmt.Println(string(decrypted))
		t.Errorf("Retrieved text does not match input!")
	}
}
