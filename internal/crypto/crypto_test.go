package crypto

import (
	"bytes"
	"testing"
)

func TestTokenGeneration(t *testing.T) {
	a, err := Token()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Token()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) < 20 {
		t.Fatalf("tokens not random/long enough")
	}
}
func TestEncryptDecrypt(t *testing.T) {
	key, _ := NewDataKey()
	p, err := Encrypt([]byte("secret"), key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(p, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret" {
		t.Fatalf("got %q", got)
	}
}
func TestPassphrase(t *testing.T) {
	h, err := HashPassphrase("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassphrase(h, "correct horse") {
		t.Fatal("expected passphrase to verify")
	}
	if VerifyPassphrase(h, "wrong") {
		t.Fatal("wrong passphrase verified")
	}
}

func TestChunkedEncryptDecryptReader(t *testing.T) {
	key, err := NewDataKey()
	if err != nil {
		t.Fatal(err)
	}
	plain := bytes.Repeat([]byte("streamed secret data"), 10000)
	var encrypted bytes.Buffer
	count, err := EncryptReader(bytes.NewReader(plain), &encrypted, key, DefaultChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(len(plain)) {
		t.Fatalf("encrypted plaintext count = %d, want %d", count, len(plain))
	}
	var decrypted bytes.Buffer
	count, err = DecryptReader(bytes.NewReader(encrypted.Bytes()), &decrypted, key)
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(len(plain)) || !bytes.Equal(decrypted.Bytes(), plain) {
		t.Fatalf("decrypted data does not match original")
	}
}

func TestChunkedDecryptRejectsTampering(t *testing.T) {
	key, _ := NewDataKey()
	var encrypted bytes.Buffer
	if _, err := EncryptReader(bytes.NewReader([]byte("tamper me")), &encrypted, key, DefaultChunkSize); err != nil {
		t.Fatal(err)
	}
	payload := encrypted.Bytes()
	payload[len(payload)-1] ^= 1
	var decrypted bytes.Buffer
	if _, err := DecryptReader(bytes.NewReader(payload), &decrypted, key); err == nil {
		t.Fatal("tampered chunk was accepted")
	}
}
