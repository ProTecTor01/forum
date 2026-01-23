package errmsg

import "testing"

func TestValidateEmail(t *testing.T) {
	if err := ValidateEmail("user@example.com"); err != nil {
		t.Fatalf("expected valid email, got error: %v", err)
	}
	if err := ValidateEmail("not-an-email"); err == nil {
		t.Fatalf("expected invalid email error")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("Aa1!aaaa"); err != nil {
		t.Fatalf("expected valid password, got error: %v", err)
	}
	if err := ValidatePassword("short"); err == nil {
		t.Fatalf("expected invalid password error")
	}
}

func TestValidateUsername(t *testing.T) {
	if err := ValidateUsername("user_name-1"); err != nil {
		t.Fatalf("expected valid username, got error: %v", err)
	}
	if err := ValidateUsername("x"); err == nil {
		t.Fatalf("expected invalid username error")
	}
}
