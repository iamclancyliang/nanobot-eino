package channels

import (
	"strings"
	"testing"
)

func TestIsSenderAllowed_EmptyDeniesAll(t *testing.T) {
	if IsSenderAllowed("test", "user1", nil) {
		t.Fatal("nil allowFrom should deny")
	}
	if IsSenderAllowed("test", "user1", []string{}) {
		t.Fatal("empty allowFrom should deny")
	}
}

func TestIsSenderAllowed_Wildcard(t *testing.T) {
	if !IsSenderAllowed("test", "anyone", []string{"*"}) {
		t.Fatal("wildcard should allow any sender")
	}
}

func TestIsSenderAllowed_ExactMatch(t *testing.T) {
	allow := []string{"ou_alice", "ou_bob"}
	if !IsSenderAllowed("test", "ou_alice", allow) {
		t.Fatal("should allow listed sender")
	}
	if !IsSenderAllowed("test", "ou_bob", allow) {
		t.Fatal("should allow listed sender")
	}
	if IsSenderAllowed("test", "ou_eve", allow) {
		t.Fatal("should deny unlisted sender")
	}
}

func TestIsSenderAllowed_WildcardAmongIDs(t *testing.T) {
	allow := []string{"ou_alice", "*"}
	if !IsSenderAllowed("test", "ou_anyone", allow) {
		t.Fatal("wildcard in list should allow any sender")
	}
}

func TestValidateAllowFrom_Empty(t *testing.T) {
	err := ValidateAllowFrom("feishu", nil)
	if err == nil {
		t.Fatal("expected error for nil allowFrom")
	}
	if !strings.Contains(err.Error(), "empty allowFrom") {
		t.Fatalf("unexpected error message: %s", err)
	}

	err = ValidateAllowFrom("feishu", []string{})
	if err == nil {
		t.Fatal("expected error for empty allowFrom")
	}
}

func TestValidateAllowFrom_NonEmpty(t *testing.T) {
	if err := ValidateAllowFrom("feishu", []string{"*"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateAllowFrom("feishu", []string{"ou_alice"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
