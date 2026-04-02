package goaladmission

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAgentDailyTokenKey_Format(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	key := AgentDailyTokenKey(id)

	// Must start with "agent:"
	if !strings.HasPrefix(key, "agent:") {
		t.Errorf("key should start with 'agent:', got %q", key)
	}

	// Must contain the agent UUID
	if !strings.Contains(key, id.String()) {
		t.Errorf("key should contain agent ID %q, got %q", id.String(), key)
	}

	// Must contain ":tokens:"
	if !strings.Contains(key, ":tokens:") {
		t.Errorf("key should contain ':tokens:', got %q", key)
	}
}

func TestAgentDailyTokenKey_IncludesCurrentDate(t *testing.T) {
	id := uuid.New()
	key := AgentDailyTokenKey(id)
	today := time.Now().UTC().Format("2006-01-02")

	if !strings.HasSuffix(key, today) {
		t.Errorf("key should end with today's date %q, got %q", today, key)
	}
}

func TestAgentDailyTokenKey_ExpectedPattern(t *testing.T) {
	id := uuid.MustParse("aabbccdd-eeff-0011-2233-445566778899")
	key := AgentDailyTokenKey(id)
	today := time.Now().UTC().Format("2006-01-02")
	want := "agent:" + id.String() + ":tokens:" + today

	if key != want {
		t.Errorf("key = %q, want %q", key, want)
	}
}

func TestAgentDailyTokenKey_DifferentAgentsDifferentKeys(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	key1 := AgentDailyTokenKey(id1)
	key2 := AgentDailyTokenKey(id2)

	if key1 == key2 {
		t.Error("different agent IDs should produce different keys")
	}
}

func TestAgentDailyTokenKey_NilUUID(t *testing.T) {
	key := AgentDailyTokenKey(uuid.Nil)
	today := time.Now().UTC().Format("2006-01-02")
	want := "agent:" + uuid.Nil.String() + ":tokens:" + today

	if key != want {
		t.Errorf("nil UUID key = %q, want %q", key, want)
	}
}

func TestErrorVariables_AreDistinct(t *testing.T) {
	errs := []error{ErrDrainMode, ErrConcurrentCap, ErrTokenBudget}
	for i, a := range errs {
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("errors[%d] and errors[%d] should be distinct", i, j)
			}
		}
	}
}

func TestErrDrainMode_Message(t *testing.T) {
	if ErrDrainMode == nil {
		t.Fatal("ErrDrainMode should not be nil")
	}
	if ErrDrainMode.Error() == "" {
		t.Error("ErrDrainMode should have a non-empty message")
	}
	if !strings.Contains(ErrDrainMode.Error(), "drain") {
		t.Errorf("ErrDrainMode message should mention 'drain', got %q", ErrDrainMode.Error())
	}
}

func TestErrConcurrentCap_Message(t *testing.T) {
	if ErrConcurrentCap == nil {
		t.Fatal("ErrConcurrentCap should not be nil")
	}
	if !strings.Contains(ErrConcurrentCap.Error(), "concurrent") {
		t.Errorf("ErrConcurrentCap message should mention 'concurrent', got %q", ErrConcurrentCap.Error())
	}
}

func TestErrTokenBudget_Message(t *testing.T) {
	if ErrTokenBudget == nil {
		t.Fatal("ErrTokenBudget should not be nil")
	}
	if !strings.Contains(ErrTokenBudget.Error(), "token") {
		t.Errorf("ErrTokenBudget message should mention 'token', got %q", ErrTokenBudget.Error())
	}
}

func TestErrorVariables_AreNotNil(t *testing.T) {
	if ErrDrainMode == nil {
		t.Error("ErrDrainMode is nil")
	}
	if ErrConcurrentCap == nil {
		t.Error("ErrConcurrentCap is nil")
	}
	if ErrTokenBudget == nil {
		t.Error("ErrTokenBudget is nil")
	}
}
