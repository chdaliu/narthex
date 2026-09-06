package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundtrip(t *testing.T) {
	hash, err := HashPassword("s3cret-密码")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "s3cret-密码") {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("garbage", "s3cret-密码") {
		t.Fatal("malformed hash accepted")
	}
}

func TestSessionRoundtrip(t *testing.T) {
	secret := "secret"
	token := Sign(secret, "abc", time.Now().Add(time.Hour).Unix())
	id, ok := Verify(secret, token)
	if !ok || id != "abc" {
		t.Fatalf("token not verified: id=%q ok=%v", id, ok)
	}
	if _, ok := Verify("other", token); ok {
		t.Fatal("token verified with wrong secret")
	}
	if _, ok := Verify(secret, token+"x"); ok {
		t.Fatal("tampered token verified")
	}
	expired := Sign(secret, "abc", time.Now().Add(-time.Hour).Unix())
	if _, ok := Verify(secret, expired); ok {
		t.Fatal("expired token verified")
	}
}

func TestRateLimiter(t *testing.T) {
	r := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !r.Allow("ip") {
			t.Fatalf("attempt %d blocked", i+1)
		}
	}
	if r.Allow("ip") {
		t.Fatal("4th attempt should be blocked")
	}
	if !r.Allow("other") {
		t.Fatal("other key should be independent")
	}
	r.Reset("ip")
	for i := 0; i < 3; i++ {
		if !r.Allow("ip") {
			t.Fatalf("after reset, attempt %d should be allowed", i+1)
		}
	}
}

func TestGeneratePassword(t *testing.T) {
	p := GeneratePassword(16)
	if len(p) != 16 {
		t.Fatalf("wrong length: %d", len(p))
	}
}
