package foggy

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
)

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

func randomID(prefix string) (string, error) {
	b, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func randomDBKey() ([]byte, error) {
	return randomBytes(32)
}

func randomSecretToken() (string, error) {
	b, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newBackupCode() (string, error) {
	b, err := randomBytes(10)
	if err != nil {
		return "", err
	}
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return raw[:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16], nil
}

func normalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, " ", "")
	code = strings.ReplaceAll(code, "-", "")
	if len(code) == 16 {
		return code[:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:]
	}
	return code
}

func deriveKey(secret string, salt []byte) []byte {
	return argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

func hashSecret(secret string, salt []byte) []byte {
	key := deriveKey(secret, salt)
	sum := sha256.Sum256(append(key, []byte("foggy-auth-verifier")...))
	return sum[:]
}

func verifySecret(secret string, salt, expected []byte) bool {
	actual := hashSecret(secret, salt)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

type wrappedKey struct {
	Salt       string `json:"salt,omitempty"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func wrapWithPassword(plain []byte, password string) (wrappedKey, error) {
	salt, err := randomBytes(16)
	if err != nil {
		return wrappedKey{}, err
	}
	key := deriveKey(password, salt)
	return wrapWithKey(plain, key, salt)
}

func unwrapWithPassword(wrapped wrappedKey, password string) ([]byte, error) {
	salt, err := base64.RawURLEncoding.DecodeString(wrapped.Salt)
	if err != nil {
		return nil, err
	}
	key := deriveKey(password, salt)
	return unwrapWithKey(wrapped, key)
}

func serverWrapKey(serverKey []byte) []byte {
	mac := hmac.New(sha256.New, serverKey)
	_, _ = mac.Write([]byte("foggy local convenience database key wrapping"))
	return mac.Sum(nil)
}

func wrapWithRawServerKey(plain []byte, serverKey []byte) (wrappedKey, error) {
	return wrapWithKey(plain, serverWrapKey(serverKey), nil)
}

func unwrapWithRawServerKey(wrapped wrappedKey, serverKey []byte) ([]byte, error) {
	return unwrapWithKey(wrapped, serverWrapKey(serverKey))
}

func wrapWithKey(plain []byte, key []byte, salt []byte) (wrappedKey, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return wrappedKey{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return wrappedKey{}, err
	}
	nonce, err := randomBytes(aead.NonceSize())
	if err != nil {
		return wrappedKey{}, err
	}
	out := wrappedKey{
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(aead.Seal(nil, nonce, plain, nil)),
	}
	if salt != nil {
		out.Salt = base64.RawURLEncoding.EncodeToString(salt)
	}
	return out, nil
}

func unwrapWithKey(wrapped wrappedKey, key []byte) ([]byte, error) {
	nonce, err := base64.RawURLEncoding.DecodeString(wrapped.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(wrapped.Ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}

func hexDBKey(key []byte) string {
	return hex.EncodeToString(key)
}

func encryptionKeyFromDBKey(dbKey []byte, purpose string) []byte {
	sum := sha256.Sum256(append(bytes.Clone(dbKey), []byte(":"+purpose)...))
	return sum[:]
}

func encryptBlob(dbKey []byte, purpose string, plain []byte) (nonce []byte, ciphertext []byte, err error) {
	block, err := aes.NewCipher(encryptionKeyFromDBKey(dbKey, purpose))
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce, err = randomBytes(aead.NonceSize())
	if err != nil {
		return nil, nil, err
	}
	return nonce, aead.Seal(nil, nonce, plain, nil), nil
}

func decryptBlob(dbKey []byte, purpose string, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(encryptionKeyFromDBKey(dbKey, purpose))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}

func mustDecodeB64(s string) ([]byte, error) {
	if s == "" {
		return nil, fmt.Errorf("empty encoded value")
	}
	return base64.RawURLEncoding.DecodeString(s)
}
