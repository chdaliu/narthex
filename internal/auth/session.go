package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// SessionCookieName is the name of the session cookie.
const SessionCookieName = "narthex_session"

// Sign creates a signed session token: base64url(id|expiryUnix).hexsig.
func Sign(secret, id string, expiry int64) string {
	payload := id + "|" + strconv.FormatInt(expiry, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signature(secret, payload)
}

// Verify checks token signature and expiry. The session id of a valid token
// is returned as ok=true.
func Verify(secret, token string) (id string, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	want := signature(secret, string(payload))
	if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(want)) != 1 {
		return "", false
	}
	idExp := strings.SplitN(string(payload), "|", 2)
	if len(idExp) != 2 {
		return "", false
	}
	exp, err := strconv.ParseInt(idExp[1], 10, 64)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() >= exp {
		return "", false
	}
	return idExp[0], true
}

func signature(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
