package util

import (
	"crypto/rand"
	"encoding/base32"
	"math/big"
	"regexp"
	"strings"
)

var EmailRe = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)

const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"

func RandID(n int) (string, error) {
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphaNum))))
		if err != nil {
			return "", err
		}
		b[i] = alphaNum[idx.Int64()]
	}
	return string(b), nil
}

func SanitizeFilename(name string) string {
	if name == "" {
		return "untitled"
	}
	r := regexp.MustCompile(`[^A-Za-z0-9_.-]`)
	return r.ReplaceAllString(name, "-")
}

func Base32NoPadding(b []byte) string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return strings.ToLower(enc.EncodeToString(b))
}
