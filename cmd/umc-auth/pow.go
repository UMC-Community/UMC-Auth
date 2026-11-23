package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type powPayload struct {
	Challenge  string `json:"c"`
	Difficulty int    `json:"d"`
	ExpiresAt  int64  `json:"e"`
	IPHash     string `json:"i"`
}

type PoW struct {
	store      *Store
	secret     []byte
	difficulty int
}

func NewPoW(store *Store, secret []byte, difficulty int) *PoW {
	return &PoW{store: store, secret: secret, difficulty: difficulty}
}

// Issue creates a short-lived signed challenge and records it for single-use
// verification.
func (p *PoW) Issue(ip string, now time.Time) (string, string, int, int64, error) {
	payload := powPayload{
		Challenge:  hex.EncodeToString(randomBytes(16)),
		Difficulty: p.difficulty,
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
		IPHash:     p.ipHash(ip),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", "", 0, 0, err
	}
	if err := p.store.StorePowChallenge(payload.Challenge, payload.ExpiresAt); err != nil {
		return "", "", 0, 0, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded + "." + p.sign(encoded), payload.Challenge, payload.Difficulty, payload.ExpiresAt, nil
}

// Verify checks the signature, IP binding, expiry, proof, and single-use
// record. The challenge is consumed only after the proof is valid.
func (p *PoW) Verify(token, nonce, ip string, now time.Time) error {
	encoded, signature, ok := strings.Cut(token, ".")
	if !ok || encoded == "" || signature == "" {
		return errors.New("invalid proof-of-work token")
	}
	if !hmac.Equal([]byte(signature), []byte(p.sign(encoded))) {
		return errors.New("invalid proof-of-work signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("invalid proof-of-work payload")
	}
	var payload powPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return errors.New("invalid proof-of-work payload")
	}
	if payload.ExpiresAt <= now.Unix() {
		return errors.New("proof-of-work challenge expired")
	}
	if !hmac.Equal([]byte(payload.IPHash), []byte(p.ipHash(ip))) {
		return errors.New("proof-of-work challenge was issued for another address")
	}
	value, err := strconv.ParseUint(nonce, 10, 64)
	if err != nil {
		return errors.New("invalid proof-of-work nonce")
	}
	if !hasLeadingZeroBits(payload.Challenge+":"+strconv.FormatUint(value, 10), payload.Difficulty) {
		return errors.New("proof-of-work answer is not sufficient")
	}
	consumed, err := p.store.ConsumePowChallenge(payload.Challenge, now.Unix())
	if err != nil {
		return fmt.Errorf("consume proof-of-work challenge: %w", err)
	}
	if !consumed {
		return errors.New("proof-of-work challenge was already used")
	}
	return nil
}

func (p *PoW) sign(encoded string) string {
	mac := hmac.New(sha256.New, p.secret)
	mac.Write([]byte(encoded))
	return hex.EncodeToString(mac.Sum(nil))
}

func (p *PoW) ipHash(ip string) string {
	mac := hmac.New(sha256.New, p.secret)
	mac.Write([]byte("ip:" + ip))
	return hex.EncodeToString(mac.Sum(nil))
}

func hasLeadingZeroBits(input string, bits int) bool {
	sum := sha256.Sum256([]byte(input))
	if bits <= 0 {
		return true
	}
	if bits > len(sum)*8 {
		return false
	}
	fullBytes := bits / 8
	for _, value := range sum[:fullBytes] {
		if value != 0 {
			return false
		}
	}
	remaining := bits % 8
	if remaining == 0 {
		return true
	}
	mask := byte(0xff << (8 - remaining))
	return sum[fullBytes]&mask == 0
}
