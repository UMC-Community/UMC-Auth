package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 19456 // KiB, OWASP baseline
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
)

func randomBytes(size int) []byte {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return bytes
}

func randomToken() string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(32))
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newUserUUID() string {
	bytes := randomBytes(16)
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // version 4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // RFC 4122 variant
	return hex.EncodeToString(bytes)
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 16 {
		return fmt.Errorf("username must be 3-16 characters")
	}
	for _, char := range username {
		if char > unicode.MaxASCII || (!unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_') {
			return fmt.Errorf("username may contain only ASCII letters, digits, and underscores")
		}
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return fmt.Errorf("password must be at most 128 characters")
	}
	return nil
}

func hashPassword(password string) (string, error) {
	salt := randomBytes(16)
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations, threads int
	for _, param := range strings.Split(parts[3], ",") {
		key, value, ok := strings.Cut(param, "=")
		if !ok {
			return false
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		switch key {
		case "m":
			memory = parsed
		case "t":
			iterations = parsed
		case "p":
			threads = parsed
		}
	}
	if memory <= 0 || iterations <= 0 || threads <= 0 || threads > 255 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual := argon2.IDKey(
		[]byte(password),
		salt,
		uint32(iterations),
		uint32(memory),
		uint8(threads),
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func prettyUUID(uuid string) string {
	normalized := strings.ReplaceAll(uuid, "-", "")
	if len(normalized) != 32 {
		return uuid
	}
	return normalized[0:8] + "-" + normalized[8:12] + "-" + normalized[12:16] + "-" +
		normalized[16:20] + "-" + normalized[20:32]
}

func normalizeUUID(uuid string) string {
	return strings.ToLower(strings.ReplaceAll(uuid, "-", ""))
}

func constantTimeEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newInviteCode() string {
	bytes := randomBytes(12)
	var builder strings.Builder
	builder.WriteString("UMC-")
	for index, value := range bytes {
		if index == 4 || index == 8 {
			builder.WriteByte('-')
		}
		builder.WriteByte(inviteAlphabet[int(value)%len(inviteAlphabet)])
	}
	return builder.String()
}
