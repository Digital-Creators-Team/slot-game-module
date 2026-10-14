package cfsign

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Signer struct {
	keyPairID  string
	privateKey *rsa.PrivateKey
	cdnDomain  string // e.g. https://dxxxx.cloudfront.net (no trailing slash)
}

type Token struct {
	Query     string `json:"query"`     // append to every asset URL: url + "?" + query
	ExpiresAt int64  `json:"expiresAt"` // unix seconds
}

func NewSigner(keyPairID, privateKeyPEM, cdnDomain string) (*Signer, error) {
	if keyPairID == "" {
		return nil, errors.New("cfsign: keyPairID is required")
	}
	if cdnDomain == "" {
		return nil, errors.New("cfsign: cdnDomain is required")
	}
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	return &Signer{
		keyPairID:  keyPairID,
		privateKey: key,
		cdnDomain:  strings.TrimRight(cdnDomain, "/"),
	}, nil
}

func NewSignerFromEnv() (*Signer, error) {
	var pemStr string
	if path := os.Getenv("CF_PRIVATE_KEY_PATH"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cfsign: read %s: %w", path, err)
		}
		pemStr = string(b)
	} else {
		pemStr = strings.ReplaceAll(os.Getenv("CF_PRIVATE_KEY"), `\n`, "\n")
	}
	return NewSigner(os.Getenv("CF_KEY_PAIR_ID"), pemStr, os.Getenv("CDN_DOMAIN"))
}

func (s *Signer) SignPrefix(prefix string, ttl time.Duration) (Token, error) {
	return s.signPrefixAt(prefix, ttl, time.Hour, time.Now())
}

func (s *Signer) signPrefixAt(prefix string, ttl, bucket time.Duration, now time.Time) (Token, error) {
	prefix = strings.Trim(prefix, "/")
	if strings.ContainsAny(prefix, "*?") {
		return Token{}, errors.New("cfsign: prefix must not contain wildcard characters")
	}
	resource := s.cdnDomain + "/*"
	if prefix != "" {
		resource = s.cdnDomain + "/" + prefix + "/*"
	}

	b := int64(bucket / time.Second)
	if b <= 0 {
		b = 1
	}
	target := now.Add(ttl).Unix()
	expires := ((target + b - 1) / b) * b // round up to the next bucket boundary

	return s.signPolicy(resource, expires)
}

// SignURL signs a single, fully qualified URL and returns it with the signature appended.
func (s *Signer) SignURL(rawURL string, ttl time.Duration) (string, error) {
	t, err := s.signPolicy(rawURL, time.Now().Add(ttl).Unix())
	if err != nil {
		return "", err
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + t.Query, nil
}

type policy struct {
	Statement []statement `json:"Statement"`
}
type statement struct {
	Resource  string    `json:"Resource"`
	Condition condition `json:"Condition"`
}
type condition struct {
	DateLessThan epochTime `json:"DateLessThan"`
}
type epochTime struct {
	EpochTime int64 `json:"AWS:EpochTime"`
}

func (s *Signer) signPolicy(resource string, expires int64) (Token, error) {
	p := policy{Statement: []statement{{
		Resource:  resource,
		Condition: condition{DateLessThan: epochTime{EpochTime: expires}},
	}}}
	policyJSON, err := json.Marshal(p)
	if err != nil {
		return Token{}, err
	}

	// CloudFront expects an RSA-SHA1 (PKCS#1 v1.5) signature over the policy JSON.
	h := sha1.Sum(policyJSON)
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA1, h[:])
	if err != nil {
		return Token{}, fmt.Errorf("cfsign: signing failed: %w", err)
	}

	q := "Policy=" + cfBase64(policyJSON) +
		"&Signature=" + cfBase64(sig) +
		"&Key-Pair-Id=" + s.keyPairID
	return Token{Query: q, ExpiresAt: expires}, nil
}

// cfBase64 applies standard base64 and then CloudFront's URL-safe substitutions:
// "+" -> "-", "=" -> "_", "/" -> "~".
func cfBase64(b []byte) string {
	return strings.NewReplacer("+", "-", "=", "_", "/", "~").
		Replace(base64.StdEncoding.EncodeToString(b))
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, errors.New("cfsign: private key is not valid PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cfsign: cannot parse private key: %w", err)
	}
	rsaKey, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("cfsign: private key must be RSA")
	}
	return rsaKey, nil
}
