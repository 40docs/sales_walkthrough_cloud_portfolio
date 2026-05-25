package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	signKey   ed25519.PrivateKey
	verifyKey ed25519.PublicKey
)

const jwtAlg = "EdDSA"

type urlClaims struct {
	jwt.RegisteredClaims
	EventID     string `json:"eid"`
	EventName   string `json:"en,omitempty"`
	PresenterID string `json:"pid,omitempty"`
	MaxUses     int    `json:"mu,omitempty"`
	Typ         string `json:"typ"`
}

type sessClaims struct {
	jwt.RegisteredClaims
	SessionID   string `json:"sid"`
	EventID     string `json:"eid"`
	PresenterID string `json:"pid,omitempty"`
	Typ         string `json:"typ"`
}

func loadKeys() error {
	v := os.Getenv("JWT_PRIVATE_KEY")
	if v == "" {
		return errors.New("JWT_PRIVATE_KEY required (base64 ed25519 seed, 32 bytes; generate with: openssl rand -base64 32)")
	}
	seed, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return err
	}
	if len(seed) != ed25519.SeedSize {
		return errors.New("JWT_PRIVATE_KEY must decode to 32 bytes")
	}
	signKey = ed25519.NewKeyFromSeed(seed)
	verifyKey = signKey.Public().(ed25519.PublicKey)
	return nil
}

func mintURLToken(eventID, eventName, presenterID string, nbf, exp time.Time, maxUses int) (string, error) {
	c := urlClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "walkthrough",
			Subject:   presenterID,
			ID:        uuid.NewString(),
			NotBefore: jwt.NewNumericDate(nbf),
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		EventID:     eventID,
		EventName:   eventName,
		PresenterID: presenterID,
		MaxUses:     maxUses,
		Typ:         "url",
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(signKey)
}

func parseURLToken(raw string) (*urlClaims, error) {
	var c urlClaims
	_, err := jwt.ParseWithClaims(raw, &c,
		func(t *jwt.Token) (interface{}, error) { return verifyKey, nil },
		jwt.WithValidMethods([]string{jwtAlg}),
	)
	if err != nil {
		return nil, err
	}
	if c.Typ != "url" {
		return nil, errors.New("not a url token")
	}
	return &c, nil
}

func mintSessionToken(sessionID, eventID, presenterID string, exp time.Time) (string, error) {
	c := sessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "walkthrough",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		SessionID:   sessionID,
		EventID:     eventID,
		PresenterID: presenterID,
		Typ:         "sess",
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, c).SignedString(signKey)
}

func parseSessionToken(raw string) (*sessClaims, error) {
	var c sessClaims
	_, err := jwt.ParseWithClaims(raw, &c,
		func(t *jwt.Token) (interface{}, error) { return verifyKey, nil },
		jwt.WithValidMethods([]string{jwtAlg}),
	)
	if err != nil {
		return nil, err
	}
	if c.Typ != "sess" {
		return nil, errors.New("not a sess token")
	}
	return &c, nil
}

// GenerateSeed prints a fresh ed25519 seed (used once at install time).
func GenerateSeed() (string, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(seed), nil
}
