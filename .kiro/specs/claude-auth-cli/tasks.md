# Implementation Plan: claude-auth CLI

## Overview

This plan addresses the identified gaps in the existing claude-auth CLI implementation and adds property-based tests using the `rapid` library to validate correctness properties defined in the design. The existing codebase is functional; these tasks focus on closing requirement gaps and adding formal verification through property tests.

## Tasks

- [x] 1. Add `rapid` dependency and fix implementation gaps
  - [x] 1.1 Add the `rapid` property-based testing library as a dependency
    - Run `go get pgregory.net/rapid` to add the rapid library to go.mod
    - _Requirements: Testing Strategy (design)_

  - [x] 1.2 Add MFA code format validation in `cmd/assume.go`
    - Before passing the MFA token code to `awscreds.AssumeRole`, validate that it is exactly 6 ASCII decimal digits
    - Return a clear error message if the format is invalid (e.g., "MFA code must be exactly 6 digits")
    - Only validate when the code comes from manual prompt (TOTP from 1Password is already validated by the provider)
    - _Requirements: 10.4_

  - [x] 1.3 Default to `claude` when no command is given to `exec`
    - In `cmd/exec.go`, when `args` is empty after stripping `--`, set `args = []string{"claude"}` instead of returning an error
    - _Requirements: 4.1_

  - [x] 1.4 Return error from `tokengen.Decode` when `X-Amz-Credential` is missing or malformed
    - In `internal/tokengen/token.go`, if `X-Amz-Credential` is empty or has fewer than 3 slash-delimited segments, return an error instead of an empty region
    - _Requirements: 9.7_

- [x] 2. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 3. Property-based tests for config and state
  - [x] 3.1 Write property test for config serialization round-trip
    - Create `internal/config/config_prop_test.go`
    - Generate random `Config` structs with non-empty required fields and session duration 1-12
    - Save via `config.Save`, load via `config.Load`, assert equality
    - Verify file permissions are 0600
    - Use `t.Setenv("HOME", t.TempDir())` for isolation
    - **Property 1: Config serialization round-trip**
    - **Validates: Requirements 1.4, 1.6**
    - _Requirements: 1.4, 1.6_

  - [x] 3.2 Write property test for state serialization round-trip
    - In `internal/config/config_prop_test.go`
    - Generate random RFC3339 timestamp strings
    - Save via `config.SaveState`, load via `config.LoadState`, assert `AnthropicTokenExpiry` matches
    - **Property 5: State serialization round-trip**
    - **Validates: Requirements 3.8**
    - _Requirements: 3.8_

- [x] 4. Property-based tests for token generation
  - [x] 4.1 Write property test for token encode/decode round-trip
    - Create `internal/tokengen/token_prop_test.go`
    - Generate random valid AWS credentials (non-empty access key, secret, session token), random valid region strings, random durations between 1s and 12h
    - Generate token via `tokengen.Generate`, decode via `tokengen.Decode`
    - Assert decoded region matches input region
    - Assert decoded expiry is within 1 second of `now + duration`
    - **Property 3: Token encode/decode round-trip**
    - **Validates: Requirements 8.5, 9.1, 9.2, 9.3**
    - _Requirements: 8.5, 9.1, 9.2, 9.3_

  - [x] 4.2 Write property test for token structural validity
    - In `internal/tokengen/token_prop_test.go`
    - Generate random valid AWS credentials and region
    - Assert token starts with `aws-external-anthropic-api-key-`
    - Assert payload after prefix is valid base64
    - Assert decoded payload contains `Action=CallWithBearerToken`, `X-Amz-Algorithm=`, `X-Amz-Credential=` with region in correct position, `X-Amz-Expires=` matching duration in seconds
    - Assert decoded payload ends with `&Version=1`
    - **Property 4: Token structural validity**
    - **Validates: Requirements 8.1, 8.2, 8.3, 8.4**
    - _Requirements: 8.1, 8.2, 8.3, 8.4_

- [x] 5. Property-based tests for CLI behavior
  - [x] 5.1 Write property test for prompt default on empty input
    - Create `cmd/setup_prop_test.go`
    - Generate random non-empty default strings and empty/whitespace-only inputs
    - Assert `prompt` returns the default value unchanged
    - **Property 2: Prompt returns default on empty input**
    - **Validates: Requirements 1.2**
    - _Requirements: 1.2_

  - [x] 5.2 Write property test for exec environment correctness
    - Create `cmd/exec_prop_test.go`
    - Extract the environment-building logic into a testable helper function
    - Generate random config values (non-empty workspace region, workspace ID) and random API key strings
    - Assert the constructed env contains exactly `CLAUDE_CODE_USE_ANTHROPIC_AWS=1`, `AWS_REGION=<region>`, `ANTHROPIC_AWS_WORKSPACE_ID=<workspace_id>`, `ANTHROPIC_AWS_API_KEY=<api_key>`
    - Assert the constructed env does NOT contain `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, or `AWS_SESSION_TOKEN` as injected variables
    - **Property 6: Exec environment correctness**
    - **Validates: Requirements 4.3, 4.5**
    - _Requirements: 4.3, 4.5_

  - [x] 5.3 Write property test for time remaining calculation
    - Create `cmd/status_prop_test.go`
    - Extract the time formatting logic into a testable helper function
    - Generate random future timestamps (1 minute to 24 hours from now)
    - Assert `displayed_hours * 60 + displayed_minutes == floor(total_minutes_remaining)`
    - **Property 7: Time remaining calculation**
    - **Validates: Requirements 5.3**
    - _Requirements: 5.3_

  - [x] 5.4 Write property test for MFA code format validation
    - Create `cmd/assume_prop_test.go`
    - Generate random strings that are NOT exactly 6 ASCII decimal digits; assert validation rejects them
    - Generate random strings that ARE exactly 6 ASCII decimal digits; assert validation accepts them
    - **Property 8: MFA code format validation**
    - **Validates: Requirements 10.4**
    - _Requirements: 10.4_

- [x] 6. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests use `pgregory.net/rapid` with minimum 100 iterations via `rapid.Check`
- The `rapid` library is the Go standard for property-based testing (similar to Hypothesis for Python)
- Implementation gaps (tasks 1.2, 1.3, 1.4) are small, targeted fixes to existing code
- Property tests validate universal correctness properties defined in the design document
- All property tests use `t.Setenv("HOME", t.TempDir())` or similar isolation to avoid touching real config

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "1.3", "1.4"] },
    { "id": 2, "tasks": ["3.1", "3.2", "4.1", "4.2", "5.1"] },
    { "id": 3, "tasks": ["5.2", "5.3", "5.4"] }
  ]
}
```
