package mfa

import (
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	tracker := NewTracker()
	if tracker == nil {
		t.Fatal("NewTracker returned nil")
	}
	if tracker.clock == nil {
		t.Fatal("NewTracker clock is nil")
	}
}

func TestNewTrackerWithClock(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 14, 0, 0, 0, time.UTC)
	tracker := NewTrackerWithClock(func() time.Time { return fixedTime })
	if tracker == nil {
		t.Fatal("NewTrackerWithClock returned nil")
	}
}

func TestShouldSkipMFA_ZeroLastMFA(t *testing.T) {
	tracker := NewTracker()
	skip, remaining := tracker.ShouldSkipMFA(time.Time{}, 60)
	if skip {
		t.Error("expected skip=false for zero lastMFA")
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0, got %v", remaining)
	}
}

func TestShouldSkipMFA_ZeroCooldown(t *testing.T) {
	tracker := NewTracker()
	skip, remaining := tracker.ShouldSkipMFA(time.Now(), 0)
	if skip {
		t.Error("expected skip=false for zero cooldown")
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0, got %v", remaining)
	}
}

func TestShouldSkipMFA_NegativeCooldown(t *testing.T) {
	tracker := NewTracker()
	skip, remaining := tracker.ShouldSkipMFA(time.Now(), -5)
	if skip {
		t.Error("expected skip=false for negative cooldown")
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0, got %v", remaining)
	}
}

func TestShouldSkipMFA_WithinCooldown(t *testing.T) {
	now := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	lastMFA := time.Date(2025, 1, 15, 14, 0, 0, 0, time.UTC)
	tracker := NewTrackerWithClock(func() time.Time { return now })

	skip, remaining := tracker.ShouldSkipMFA(lastMFA, 60)
	if !skip {
		t.Error("expected skip=true when within cooldown")
	}
	expected := 30 * time.Minute
	if remaining != expected {
		t.Errorf("expected remaining=%v, got %v", expected, remaining)
	}
}

func TestShouldSkipMFA_ExactlyAtCooldown(t *testing.T) {
	now := time.Date(2025, 1, 15, 15, 0, 0, 0, time.UTC)
	lastMFA := time.Date(2025, 1, 15, 14, 0, 0, 0, time.UTC)
	tracker := NewTrackerWithClock(func() time.Time { return now })

	skip, remaining := tracker.ShouldSkipMFA(lastMFA, 60)
	if skip {
		t.Error("expected skip=false when exactly at cooldown boundary")
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0, got %v", remaining)
	}
}

func TestShouldSkipMFA_PastCooldown(t *testing.T) {
	now := time.Date(2025, 1, 15, 16, 0, 0, 0, time.UTC)
	lastMFA := time.Date(2025, 1, 15, 14, 0, 0, 0, time.UTC)
	tracker := NewTrackerWithClock(func() time.Time { return now })

	skip, remaining := tracker.ShouldSkipMFA(lastMFA, 60)
	if skip {
		t.Error("expected skip=false when past cooldown")
	}
	if remaining != 0 {
		t.Errorf("expected remaining=0, got %v", remaining)
	}
}

func TestRecordMFA(t *testing.T) {
	fixedTime := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	tracker := NewTrackerWithClock(func() time.Time { return fixedTime })

	result := tracker.RecordMFA()
	expected := "2025-01-15T14:30:00Z"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestParseMFATimestamp_Valid(t *testing.T) {
	result := ParseMFATimestamp("2025-01-15T14:30:00Z")
	expected := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestParseMFATimestamp_Empty(t *testing.T) {
	result := ParseMFATimestamp("")
	if !result.IsZero() {
		t.Errorf("expected zero time for empty string, got %v", result)
	}
}

func TestParseMFATimestamp_Invalid(t *testing.T) {
	result := ParseMFATimestamp("not-a-timestamp")
	if !result.IsZero() {
		t.Errorf("expected zero time for invalid string, got %v", result)
	}
}

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0h 0m"},
		{"negative", -5 * time.Minute, "0h 0m"},
		{"one minute", 1 * time.Minute, "0h 1m"},
		{"30 minutes", 30 * time.Minute, "0h 30m"},
		{"59 minutes", 59 * time.Minute, "0h 59m"},
		{"60 minutes", 60 * time.Minute, "1h 0m"},
		{"90 minutes", 90 * time.Minute, "1h 30m"},
		{"2 hours 15 minutes", 135 * time.Minute, "2h 15m"},
		{"truncates seconds", 90*time.Minute + 45*time.Second, "1h 30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRemaining(tt.duration)
			if result != tt.expected {
				t.Errorf("FormatRemaining(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestValidateCooldownMinutes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"zero", "0", 0, false},
		{"positive", "60", 60, false},
		{"large", "1440", 1440, false},
		{"negative", "-1", 0, true},
		{"decimal", "1.5", 0, true},
		{"empty", "", 0, true},
		{"text", "abc", 0, true},
		{"spaces around valid", " 60 ", 60, false},
		{"mixed", "60abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateCooldownMinutes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCooldownMinutes(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ValidateCooldownMinutes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
