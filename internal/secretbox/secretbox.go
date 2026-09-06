// Package secretbox encrypts small secrets at rest (currently: the login
// username) with AES-256-GCM. The key is derived from the session secret
// via HMAC-SHA256, so it lives inside the same config file — this protects
// against casually reading the config, not against an attacker who holds
// the whole file. The password itself is never stored here: it is an
// argon2id hash (one-way) in the config.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const context = "narthex:username:v1"

// DeriveKey derives the AES key from the session secret.
func DeriveKey(secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(context))
	return mac.Sum(nil)
}

// Encrypt seals plaintext with a fresh random nonce and returns
// base64(nonce || ciphertext).
func Encrypt(secret, plaintext string) (string, error) {
	block, err := aes.NewCipher(DeriveKey(secret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Decrypt opens a blob produced by Encrypt.
func Decrypt(secret, blob string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(DeriveKey(secret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
