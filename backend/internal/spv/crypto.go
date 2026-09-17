package spv

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

type AESGCMCipher struct{ key []byte }

func NewAESGCMCipher(encoded string) (*AESGCMCipher, error) {
	key, err := hex.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("SPV token encryption key must be 64 hexadecimal characters")
	}
	return &AESGCMCipher{key: key}, nil
}

func (c *AESGCMCipher) Encrypt(value string) (string, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), nil)), nil
}

func (c *AESGCMCipher) Decrypt(value string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode encrypted SPV token: %w", err)
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errorsNewCiphertext()
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", errorsNewCiphertext()
	}
	return string(plain), nil
}

func errorsNewCiphertext() error { return fmt.Errorf("invalid encrypted SPV token") }
