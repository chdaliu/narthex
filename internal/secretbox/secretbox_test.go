package secretbox

import (
	"strings"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	for _, secret := range []string{"", "secret", "a-very-long-secret-that-exceeds-32-bytes-0123456789"} {
		blob, err := Encrypt(secret, "narthex")
		if err != nil {
			t.Fatal(err)
		}
		if blob == "" || strings.Contains(blob, "narthex") {
			t.Fatalf("blob should not leak plaintext: %q", blob)
		}
		got, err := Decrypt(secret, blob)
		if err != nil || got != "narthex" {
			t.Fatalf("roundtrip failed: %q err=%v", got, err)
		}
	}
}

func TestWrongSecretFails(t *testing.T) {
	blob, err := Encrypt("secret-a", "narthex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt("secret-b", blob); err == nil {
		t.Fatal("decrypt with wrong secret should fail")
	}
}

func TestTamperedBlobFails(t *testing.T) {
	blob, err := Encrypt("secret", "narthex")
	if err != nil {
		t.Fatal(err)
	}
	tampered := blob[:len(blob)-2] + "AA"
	if _, err := Decrypt("secret", tampered); err == nil {
		t.Fatal("tampered blob should fail")
	}
	if _, err := Decrypt("secret", "not-base64!!!"); err == nil {
		t.Fatal("invalid base64 should fail")
	}
}

func TestUniqueNonce(t *testing.T) {
	a, err := Encrypt("secret", "narthex")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt("secret", "narthex")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two encryptions of the same value should differ")
	}
}
