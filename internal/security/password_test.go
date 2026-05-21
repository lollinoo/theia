package security

import (
	"strings"
	"testing"
)

func TestPasswordHashingVerifiesAndDoesNotExposePlaintext(t *testing.T) {
	password := "Correct Horse Battery Staple 2026!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == password {
		t.Fatal("password hash equals plaintext password")
	}
	if strings.Contains(hash, password) {
		t.Fatal("password hash contains plaintext password")
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("password hash = %q, want argon2id format", hash)
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("VerifyPassword correct password: %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword rejected the correct password")
	}

	ok, err = VerifyPassword("Wrong Horse Battery Staple 2026!", hash)
	if err != nil {
		t.Fatalf("VerifyPassword wrong password: %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword accepted the wrong password")
	}
}

func TestPasswordPolicyRejectsWeakPasswordsAndAllowsLongPassphrases(t *testing.T) {
	for _, password := range []string{
		"short",
		"password",
		"theia",
		"administrator",
		"123456789012",
		strings.Repeat("a", MaxPasswordLength+1),
	} {
		if err := ValidatePasswordPolicy(password); err == nil {
			t.Fatalf("ValidatePasswordPolicy(%q) returned nil error", password)
		}
	}

	longPassphrase := strings.Repeat("correct horse battery staple ", 20)
	if err := ValidatePasswordPolicy(longPassphrase); err != nil {
		t.Fatalf("ValidatePasswordPolicy(long passphrase): %v", err)
	}
}

func TestPasswordTokenHashingUsesSecretAndDoesNotExposeToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned an empty token")
	}

	firstHash := HashToken(token, []byte("first-session-secret"))
	secondHash := HashToken(token, []byte("first-session-secret"))
	otherSecretHash := HashToken(token, []byte("second-session-secret"))

	if firstHash == token {
		t.Fatal("token hash equals plaintext token")
	}
	if strings.Contains(firstHash, token) {
		t.Fatal("token hash contains plaintext token")
	}
	if firstHash != secondHash {
		t.Fatal("HashToken was not deterministic for the same token and secret")
	}
	if firstHash == otherSecretHash {
		t.Fatal("HashToken ignored the caller-provided secret")
	}
}
