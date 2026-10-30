package bootstrap

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSecretRequiresStrongShortLivedSecret(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		secret    string
		expiresAt time.Time
		wantErr   bool
	}{
		{name: "valid", secret: strings.Repeat("x", 32), expiresAt: now.Add(30 * time.Minute)},
		{name: "short secret", secret: "short", expiresAt: now.Add(30 * time.Minute), wantErr: true},
		{name: "expired", secret: strings.Repeat("x", 32), expiresAt: now.Add(-time.Minute), wantErr: true},
		{name: "too long lifetime", secret: strings.Repeat("x", 32), expiresAt: now.Add(2 * time.Hour), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSecret(test.secret, test.expiresAt, now); (err != nil) != test.wantErr {
				t.Fatalf("validateSecret() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateAdminRequiresValidProfileAndStrongPassword(t *testing.T) {
	valid := InitialAdmin{
		StaffID:     "ADMIN-001",
		Email:       "admin@example.test",
		DisplayName: "Initial Administrator",
		Password:    "C0mpl3x-password",
	}
	if err := validateAdmin(valid); err != nil {
		t.Fatalf("validateAdmin(valid) error = %v", err)
	}

	invalidPassword := valid
	invalidPassword.Password = "weakpassword"
	if err := validateAdmin(invalidPassword); err == nil {
		t.Fatal("validateAdmin accepted a weak password")
	}

	invalidEmail := valid
	invalidEmail.Email = "not-an-email"
	if err := validateAdmin(invalidEmail); err == nil {
		t.Fatal("validateAdmin accepted an invalid email")
	}
}

func TestHashSecretDoesNotReturnTheSecret(t *testing.T) {
	secret := strings.Repeat("s", minimumSecretLength)
	if got := hashSecret(secret); got == secret || len(got) != 64 {
		t.Fatalf("hashSecret returned an invalid digest")
	}
}
