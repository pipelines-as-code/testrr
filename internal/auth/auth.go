package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"testrr/internal/store"
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("%s:%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func ComparePassword(hashedPassword, candidate string) bool {
	parts := strings.Split(hashedPassword, ":")
	if len(parts) != 2 {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	expected, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	computed := argon2.IDKey([]byte(candidate), salt, 1, 64*1024, 4, uint32(len(expected)))
	return subtle.ConstantTimeCompare(expected, computed) == 1
}

func AuthenticateBasic(ctx context.Context, repository store.Repository, slug, username, password string) (store.Project, error) {
	credential, err := repository.GetProjectCredential(ctx, slug, username)
	if err != nil {
		return store.Project{}, err
	}
	if !ComparePassword(credential.PasswordHash, password) {
		return store.Project{}, store.ErrNotFound
	}
	return credential.Project, nil
}
