# Implementation Plan: MFA Rate-Limiting

## Overview

Implement MFA rate-limiting for the `claude-auth` CLI tool. This adds cooldown-based MFA skipping to the credential refresh flow, reducing TOTP prompts for developers who refresh frequently. The implementation creates a new `internal/mfa` package for cooldown logic, extends the config/state models, and modifies the `refresh`, `setup`, `status`, and `clear` commands.

## Tasks

- [x] 1. Create `internal/mfa` package with core cooldown logic
  - [x] 1.1 Implement `Tracker` struct, `ShouldSkipMFA`, `RecordMFA`, `ParseMFATimestamp`, `FormatRemaining`, and `ValidateCooldownMinutes` functions
    - Create `internal/mfa/mfa.go`
    - `Tracker` struct with injectable clock (`func() time.Time`)
    - `NewTracker()` and `NewTrackerWithClock(clock)` constructors
    - `ShouldSkipMFA(lastMFA time.Time, cooldownMinutes int) (bool, time.Duration)` — returns true only if lastMFA is non-zero, cooldown > 0, and elapsed < cooldown
    - `RecordMFA() string` — returns current UTC time as RFC3339
    - `ParseMFATimestamp(s string) time.Time` — returns zero time on empty/invalid input
    - `FormatRemaining(d time.Duration) string` — formats as "Xh Ym" with truncation
    - `ValidateCooldownMinutes(input string) (int, error)` — validates non-negative integer input
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 3.4, 3.5, 3.6_

  - [x] 1.2 Write property test for MFA cooldown decision correctness
    - **Property 1: MFA cooldown decision correctness**
    - Generate random (lastMFA, now, cooldownMinutes) tuples and verify skip decision matches spec
    - **Validates: Requirements 2.2, 2.3, 3.3**

  - [x] 1.3 Write property test for cooldown time remaining formatting
    - **Property 3: Cooldown time remaining formatting**
    - Generate random durations and verify output format "Xh Ym" and arithmetic correctness (X*60 + Y == truncated total minutes)
    - **Validates: Requirements 2.6, 5.1**

  - [x] 1.4 Write property test for cooldown configuration validation
    - **Property 4: Cooldown configuration validation**
    - Generate random strings (valid integers, negatives, decimals, text) and verify acceptance/rejection
    - **Validates: Requirements 3.4, 3.5, 3.6**

- [x] 2. Extend config and state models
  - [x] 2.1 Add `MFACooldownMinutes` field to `Config` and `LastMFASuccess` field to `State` in `internal/config/config.go`
    - Add `MFACooldownMinutes *int \`json:"mfa_cooldown_minutes,omitempty"\`` to `Config` struct
    - Add `GetMFACooldownMinutes() int` method that returns 60 when field is nil
    - Add `LastMFASuccess string \`json:"last_mfa_success,omitempty"\`` to `State` struct
    - _Requirements: 1.4, 3.1_

  - [x] 2.2 Write property test for config default cooldown value
    - **Property 5: Config default value for missing cooldown field**
    - Generate valid config JSON without `mfa_cooldown_minutes` and verify default is 60
    - **Validates: Requirements 3.1**

- [x] 3. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Modify `cmd/refresh.go` to integrate MFA cooldown logic
  - [x] 4.1 Add `--force-mfa` flag and MFA cooldown evaluation to the refresh command
    - Add `--force-mfa` boolean flag (default false) to `refreshCmd`
    - Before calling `assumeConfiguredRole`: check `--force-mfa`, then evaluate cooldown via `mfa.Tracker.ShouldSkipMFA`
    - If `--force-mfa` and MFA serial is empty, return error: "--force-mfa requires MFA to be configured"
    - If `--force-mfa` and cooldown active, display message that MFA is being forced
    - If cooldown active (skip=true), display skip message with time remaining using `mfa.FormatRemaining`
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 2.1, 2.2, 2.3, 2.6_

  - [x] 4.2 Implement AccessDenied fallback and MFA timestamp recording in refresh flow
    - When MFA is skipped (cooldown active): call `assumeRoleWithoutMFA`, if AccessDenied → display fallback message, perform MFA, retry once
    - On non-AccessDenied error from MFA-less attempt: return error immediately (no fallback)
    - On successful MFA authentication: record timestamp via `mfa.Tracker.RecordMFA()` and persist to state
    - On successful MFA-less assumption: do NOT update MFA timestamp
    - If state file write fails: log warning to stderr, continue
    - _Requirements: 1.1, 1.2, 1.3, 6.1, 6.2, 6.3, 6.4, 6.5_

  - [x] 4.3 Write property test for MFA timestamp recording invariant
    - **Property 2: MFA timestamp recording invariant**
    - Generate random assume scenarios (with/without token code, with/without pre-existing timestamp) and verify state mutation rules
    - **Validates: Requirements 1.1, 1.2**

  - [x] 4.4 Write property test for non-AccessDenied errors bypassing fallback
    - **Property 6: Non-AccessDenied errors bypass fallback**
    - Generate random non-AccessDenied error types and verify no fallback is triggered
    - **Validates: Requirements 6.4**

- [x] 5. Refactor `cmd/assume.go` to support MFA-less role assumption
  - [x] 5.1 Split `assumeConfiguredRole` into MFA and non-MFA variants
    - Create `assumeRoleWithoutMFA(ctx, cfg)` that calls `awscreds.AssumeRole` with empty MFA serial and token code
    - Modify existing `assumeConfiguredRole` to remain the MFA path (fetches TOTP from 1Password or prompts)
    - Both functions share the 1Password credential fetch for access key/secret
    - _Requirements: 2.2, 6.1, 2.7_

- [x] 6. Modify `cmd/setup.go` to prompt for MFA cooldown
  - [x] 6.1 Add MFA cooldown prompt to the setup wizard when MFA serial is non-empty
    - After MFA serial prompt: if non-empty, prompt for cooldown minutes with default 60
    - Validate input using `mfa.ValidateCooldownMinutes`; re-prompt on invalid input (negative or non-integer)
    - Store validated value in `cfg.MFACooldownMinutes`
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.6_

- [x] 7. Modify `cmd/status.go` to display MFA cooldown status
  - [x] 7.1 Add MFA cooldown status display to the status command
    - If MFA serial is empty: omit MFA cooldown info entirely
    - If `mfa_cooldown_minutes` is 0: display "MFA rate-limiting is disabled"
    - If no MFA timestamp or invalid timestamp: display "MFA has not been used yet"
    - If within cooldown: display time remaining in "Xh Ym" format
    - If cooldown expired: display "Next refresh will require MFA"
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_

- [x] 8. Modify `cmd/clear.go` to update confirmation message
  - [x] 8.1 Update the clear command confirmation message to mention MFA state
    - Change success message to indicate both token and MFA state have been cleared
    - No logic change needed — state.json already contains the MFA timestamp and is already deleted
    - _Requirements: 7.1, 7.4_

- [x] 9. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 10. Integration wiring and final validation
  - [x] 10.1 Wire the complete refresh flow end-to-end and verify state file permissions
    - Ensure state file is created with 0600 permissions and directory with 0700 (already handled by `config.SaveState` but verify)
    - Verify `clear` → `refresh` requires MFA (no timestamp in state)
    - Verify the full flow: refresh with MFA → refresh within cooldown (skip) → refresh after cooldown (MFA again)
    - _Requirements: 1.5, 7.2, 7.3_

  - [x] 10.2 Write unit tests for integration scenarios
    - Test: `--force-mfa` with empty MFA serial returns error
    - Test: AccessDenied triggers exactly one MFA retry
    - Test: Fallback MFA failure returns second error
    - Test: Status display messages for each state
    - Test: Setup wizard prompts for cooldown when MFA serial is non-empty
    - _Requirements: 4.5, 6.1, 6.3, 5.1, 5.2, 5.3, 5.4, 3.2_

- [x] 11. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The project uses `pgregory.net/rapid` for property-based testing
- All property tests should be placed in `internal/mfa/mfa_prop_test.go` (for Properties 1, 3, 4, 5) and `cmd/refresh_prop_test.go` (for Properties 2, 6)

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1"] },
    { "id": 1, "tasks": ["1.2", "1.3", "1.4", "2.2", "5.1"] },
    { "id": 2, "tasks": ["4.1", "6.1", "7.1", "8.1"] },
    { "id": 3, "tasks": ["4.2"] },
    { "id": 4, "tasks": ["4.3", "4.4", "10.1"] },
    { "id": 5, "tasks": ["10.2"] }
  ]
}
```
