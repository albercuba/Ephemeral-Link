package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const TokenBytes = 24

type EncryptedPayload struct { Nonce, Ciphertext string }
type WrappedKey struct { Nonce, Ciphertext string }

func RandomBytes(n int) ([]byte, error) { b := make([]byte, n); _, err := io.ReadFull(rand.Reader, b); return b, err }
func Token() (string, error) { b, err := RandomBytes(TokenBytes); if err != nil { return "", err }; return base64.RawURLEncoding.EncodeToString(b), nil }

func Encrypt(plaintext, key []byte) (EncryptedPayload, error) {
	block, err := aes.NewCipher(key); if err != nil { return EncryptedPayload{}, err }
	gcm, err := cipher.NewGCM(block); if err != nil { return EncryptedPayload{}, err }
	nonce, err := RandomBytes(gcm.NonceSize()); if err != nil { return EncryptedPayload{}, err }
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return EncryptedPayload{Nonce: b64(nonce), Ciphertext: b64(ct)}, nil
}

func Decrypt(p EncryptedPayload, key []byte) ([]byte, error) {
	nonce, err := ub64(p.Nonce); if err != nil { return nil, err }
	ct, err := ub64(p.Ciphertext); if err != nil { return nil, err }
	block, err := aes.NewCipher(key); if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(block); if err != nil { return nil, err }
	return gcm.Open(nil, nonce, ct, nil)
}

func NewDataKey() ([]byte, error) { return RandomBytes(32) }
func WrapKey(dataKey, master []byte) (WrappedKey, error) { p, err := Encrypt(dataKey, master); return WrappedKey(p), err }
func UnwrapKey(w WrappedKey, master []byte) ([]byte, error) { return Decrypt(EncryptedPayload(w), master) }

func HashPassphrase(pass string) (string, error) {
	pass = strings.TrimSpace(pass)
	if pass == "" { return "", nil }
	salt, err := RandomBytes(16); if err != nil { return "", err }
	h := argon2.IDKey([]byte(pass), salt, 1, 64*1024, 4, 32)
	return b64(salt)+":"+b64(h), nil
}

func VerifyPassphrase(encoded, pass string) bool {
	if encoded == "" { return true }
	parts := strings.Split(encoded, ":"); if len(parts) != 2 { return false }
	salt, err := ub64(parts[0]); if err != nil { return false }
	want, err := ub64(parts[1]); if err != nil { return false }
	got := argon2.IDKey([]byte(strings.TrimSpace(pass)), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
func ub64(s string) ([]byte, error) { if s == "" { return nil, errors.New("empty base64") }; return base64.StdEncoding.DecodeString(s) }
