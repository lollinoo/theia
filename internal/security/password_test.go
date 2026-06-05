package security

import (
	"encoding/base64"
	"fmt"
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

func TestPasswordVerifyRejectsMalformedAndUnsupportedHashes(t *testing.T) {
	cases := map[string]string{
		"wrong algorithm":     strings.Replace(testArgon2idHash(65536, 3, 4, 16, 32), "$argon2id$", "$argon2i$", 1),
		"zero memory":         testArgon2idHash(0, 3, 4, 16, 32),
		"zero iterations":     testArgon2idHash(65536, 0, 4, 16, 32),
		"zero parallelism":    testArgon2idHash(65536, 3, 0, 16, 32),
		"unsupported memory":  testArgon2idHash(65535, 3, 4, 16, 32),
		"huge memory":         testArgon2idHash(1<<30, 3, 4, 16, 32),
		"unsupported passes":  testArgon2idHash(65536, 4, 4, 16, 32),
		"unsupported lanes":   testArgon2idHash(65536, 3, 5, 16, 32),
		"unexpected salt len": testArgon2idHash(65536, 3, 4, 15, 32),
		"unexpected hash len": testArgon2idHash(65536, 3, 4, 16, 31),
		"extra param":         "$argon2id$v=19$m=65536,t=3,p=4,x=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}

	for name, hash := range cases {
		t.Run(name, func(t *testing.T) {
			ok, err := verifyPasswordNoPanic(t, "Correct Horse Battery Staple 2026!", hash)
			if err == nil {
				t.Fatalf("VerifyPassword returned nil error for hostile hash %q", hash)
			}
			if ok {
				t.Fatal("VerifyPassword accepted a hostile hash")
			}
		})
	}
}

func TestPasswordPolicyRequiresUserFriendlyComplexityRules(t *testing.T) {
	for _, tt := range []struct {
		name     string
		password string
		want     string
	}{
		{name: "too short", password: "Aa1!short", want: "10 to 24 characters"},
		{name: "too long", password: "Aa1!" + strings.Repeat("x", 21), want: "10 to 24 characters"},
		{name: "missing uppercase", password: "lowercase1!", want: "uppercase letter"},
		{name: "missing lowercase", password: "UPPERCASE1!", want: "lowercase letter"},
		{name: "missing number", password: "NoNumber!!", want: "number"},
		{name: "missing special", password: "NoSpecial12", want: "special character"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordPolicy(tt.password)
			if err == nil {
				t.Fatalf("ValidatePasswordPolicy(%q) returned nil error", tt.password)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidatePasswordPolicy(%q) error = %q, want %q", tt.password, err, tt.want)
			}
		})
	}

	for _, password := range []string{
		"ValidPass1!",
		"   ValidPass1!   ",
		"Password123!",
	} {
		t.Run("accepts "+password, func(t *testing.T) {
			if err := ValidatePasswordPolicy(password); err != nil {
				t.Fatalf("ValidatePasswordPolicy(%q): %v", password, err)
			}
		})
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

func testArgon2idHash(memory uint32, iterations uint32, parallelism uint8, saltLen int, hashLen int) string {
	salt := make([]byte, saltLen)
	hash := make([]byte, hashLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func verifyPasswordNoPanic(t *testing.T, password string, hash string) (ok bool, err error) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("VerifyPassword panicked for hostile hash: %v", recovered)
		}
	}()
	return VerifyPassword(password, hash)
}
