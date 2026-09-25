package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const TokenBytes = 24
const DefaultChunkSize = 64 * 1024

var ChunkedMagic = []byte("ELCHUNK1")

type EncryptedPayload struct{ Nonce, Ciphertext string }
type WrappedKey struct{ Nonce, Ciphertext string }

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	return b, err
}
func Token() (string, error) {
	b, err := RandomBytes(TokenBytes)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func Encrypt(plaintext, key []byte) (EncryptedPayload, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedPayload{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedPayload{}, err
	}
	nonce, err := RandomBytes(gcm.NonceSize())
	if err != nil {
		return EncryptedPayload{}, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return EncryptedPayload{Nonce: b64(nonce), Ciphertext: b64(ct)}, nil
}

func EncryptReader(src io.Reader, dst io.Writer, key []byte, chunkSize int) (int64, error) {
	if chunkSize <= 0 || chunkSize > DefaultChunkSize {
		chunkSize = DefaultChunkSize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}
	if _, err := dst.Write(ChunkedMagic); err != nil {
		return 0, err
	}
	buffer := make([]byte, chunkSize)
	var total int64
	var index uint64
	for {
		n, readErr := io.ReadFull(src, buffer)
		if readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return total, readErr
		}
		nonce, err := RandomBytes(gcm.NonceSize())
		if err != nil {
			return total, err
		}
		aad := make([]byte, 8)
		binary.BigEndian.PutUint64(aad, index)
		ciphertext := gcm.Seal(nil, nonce, buffer[:n], aad)
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(n))
		if _, err := dst.Write(length[:]); err != nil {
			return total, err
		}
		if _, err := dst.Write(nonce); err != nil {
			return total, err
		}
		if _, err := dst.Write(ciphertext); err != nil {
			return total, err
		}
		total += int64(n)
		index++
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	return total, nil
}

func DecryptReader(src io.Reader, dst io.Writer, key []byte) (int64, error) {
	magic := make([]byte, len(ChunkedMagic))
	if _, err := io.ReadFull(src, magic); err != nil {
		return 0, err
	}
	if string(magic) != string(ChunkedMagic) {
		return 0, errors.New("invalid chunked payload header")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}
	var total int64
	var index uint64
	for {
		var length [4]byte
		n, err := io.ReadFull(src, length[:])
		if err == io.EOF && n == 0 {
			return total, nil
		}
		if err != nil {
			return total, errors.New("truncated chunked payload")
		}
		plainLen := binary.BigEndian.Uint32(length[:])
		if plainLen == 0 || plainLen > DefaultChunkSize {
			return total, errors.New("invalid chunked payload length")
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(src, nonce); err != nil {
			return total, errors.New("truncated chunked payload nonce")
		}
		ciphertext := make([]byte, int(plainLen)+gcm.Overhead())
		if _, err := io.ReadFull(src, ciphertext); err != nil {
			return total, errors.New("truncated chunked payload ciphertext")
		}
		aad := make([]byte, 8)
		binary.BigEndian.PutUint64(aad, index)
		plain, err := gcm.Open(nil, nonce, ciphertext, aad)
		if err != nil {
			return total, errors.New("invalid chunked payload authentication")
		}
		if _, err := dst.Write(plain); err != nil {
			return total, err
		}
		total += int64(len(plain))
		index++
	}
}

func Decrypt(p EncryptedPayload, key []byte) ([]byte, error) {
	nonce, err := ub64(p.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := ub64(p.Ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func NewDataKey() ([]byte, error) { return RandomBytes(32) }
func WrapKey(dataKey, master []byte) (WrappedKey, error) {
	p, err := Encrypt(dataKey, master)
	return WrappedKey(p), err
}
func UnwrapKey(w WrappedKey, master []byte) ([]byte, error) {
	return Decrypt(EncryptedPayload(w), master)
}

func HashPassphrase(pass string) (string, error) {
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return "", nil
	}
	salt, err := RandomBytes(16)
	if err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(pass), salt, 1, 64*1024, 4, 32)
	return b64(salt) + ":" + b64(h), nil
}

func VerifyPassphrase(encoded, pass string) bool {
	if encoded == "" {
		return true
	}
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := ub64(parts[0])
	if err != nil {
		return false
	}
	want, err := ub64(parts[1])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(strings.TrimSpace(pass)), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
func ub64(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("empty base64")
	}
	return base64.StdEncoding.DecodeString(s)
}
