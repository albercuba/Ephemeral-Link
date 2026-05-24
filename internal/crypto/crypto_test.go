package crypto

import "testing"

func TestTokenGeneration(t *testing.T) { a, err := Token(); if err != nil { t.Fatal(err) }; b, err := Token(); if err != nil { t.Fatal(err) }; if a == b || len(a) < 20 { t.Fatalf("tokens not random/long enough") } }
func TestEncryptDecrypt(t *testing.T) { key, _ := NewDataKey(); p, err := Encrypt([]byte("secret"), key); if err != nil { t.Fatal(err) }; got, err := Decrypt(p, key); if err != nil { t.Fatal(err) }; if string(got) != "secret" { t.Fatalf("got %q", got) } }
func TestPassphrase(t *testing.T) { h, err := HashPassphrase("correct horse"); if err != nil { t.Fatal(err) }; if !VerifyPassphrase(h, "correct horse") { t.Fatal("expected passphrase to verify") }; if VerifyPassphrase(h, "wrong") { t.Fatal("wrong passphrase verified") } }
