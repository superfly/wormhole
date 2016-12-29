package smux

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/nacl/box"
)

func newKXRFrame(data []byte) Frame {
	f := newFrame(cmdKXR, 0)
	f.data = data
	return f
}

func newKXSFrame(data []byte) Frame {
	f := newFrame(cmdKXS, 0)
	f.data = data
	return f
}

func newKeyPair() (publicKey, privateKey *[32]byte, err error) {
	return box.GenerateKey(rand.Reader)
}

func newSecret(privateKey, peersPublicKey *[32]byte) *[32]byte {
	var secret [32]byte
	box.Precompute(&secret, peersPublicKey, privateKey)
	return &secret
}

func sealSecret(secret, publicKey *[32]byte) ([]byte, error) {
	var nonce [24]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil, err
	}

	encrypted := box.SealAfterPrecomputation(nonce[:], secret[:], &nonce, secret)
	msg := make([]byte, len(encrypted)+32)
	copy(msg[:32], publicKey[:])
	copy(msg[32:], encrypted)
	return msg, nil
}

func verifyKeyExchange(privKey *[32]byte, data []byte) (*[32]byte, error) {
	// msg must include:
	// nonce (24 bytes), session public key (32 bytes), encrypted shared key (at least 32 bytes)
	if len(data) < 24+32+32 {
		return nil, errors.New(errBadKeyExchange)
	}

	var nonce [24]byte
	var sessionPublicKey [32]byte
	copy(sessionPublicKey[:], data[:32])
	copy(nonce[:], data[32:24+32])
	decrypted, ok := box.Open([]byte{}, data[24+32:], &nonce, &sessionPublicKey, privKey)
	if !ok || len(decrypted) < 32 {
		return nil, errors.New(errBadKey)
	}
	var sharedKey [32]byte
	copy(sharedKey[:], decrypted)
	return &sharedKey, nil
}

func decrypt(s *Session, dst []byte, src []byte) error {
	/*tmp := make([]byte, len(src))
	s.cryptStreamLock.Lock()
	defer s.cryptStreamLock.Unlock()
	if s.cryptStream == nil {
		return errors.New(errNoEncryptionKey)
	}
	*/
	s.cryptStreamLock.Lock()
	defer s.cryptStreamLock.Unlock()
	stream, err := newCipherStream(s.encryptionKey)
	if err != nil {
		return err
	}
	stream.XORKeyStream(dst, src)
	return nil
}

func encrypt(s *Session, dst []byte, src []byte) error {
	// encrypt and decrypt are symmetric
	return decrypt(s, dst, src)
}

func newCipherStream(key *[32]byte) (cipher.Stream, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	// If the key is unique for each ciphertext, then it's ok to use a zero IV.
	var iv [aes.BlockSize]byte
	return cipher.NewOFB(block, iv[:]), nil
}
