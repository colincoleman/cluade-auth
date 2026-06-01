# Requirements Document

## Introduction

MFA rate-limiting prevents unnecessary MFA prompts during credential refresh by tracking when MFA was last successfully used and skipping the MFA step if the previous authentication occurred within a configurable cooldown window (default: 1 hour). When MFA is skipped, the tool assumes the role without MFA parameters, relying on the fact that the role trust policy allows MFA-less assumption from the same IAM user when a valid session already exists, or that the user accepts a shorter session without MFA. This reduces friction for developers who refresh credentials multiple times per day without compromising security posture.

## Glossary

- **CLI**: The claude-auth command-line interface application
- **MFA_Tracker**: The component responsible for recording and querying the timestamp of the last successful MFA authentication
- **Role_Assumer**: The component that calls AWS STS AssumeRole with long-term credentials and optionally MFA to obtain short-term session credentials
- **1Password_Client**: The integration layer that communicates with the 1Password desktop application via its SDK to read credentials and TOTP codes
- **Config_Manager**: The component responsible for loading, saving, and validating configuration from `~/.config/claude-auth/config.json`
- **Credential_Store**: The local file system storage at `~/.config/claude-auth/` holding state and credential files
- **MFA_Cooldown**: The minimum duration (in minutes) that must elapse after a successful MFA authentication before the system requires MFA again (default: 60 minutes)
- **State_File**: The JSON file at `~/.config/claude-auth/state.json` that persists runtime state including token expiry and MFA timestamps

## Requirements

### Requirement 1: MFA Timestamp Recording

**User Story:** As a developer, I want the tool to record when I last successfully authenticated with MFA, so that it can determine whether a subsequent refresh needs MFA again.

#### Acceptance Criteria

1. WHEN STS AssumeRole succeeds with a non-empty MFA token code provided, THE MFA_Tracker SHALL record the current UTC timestamp in RFC3339 format as the last successful MFA time in the State_File, persisted in the same write operation as the token expiry field
2. WHEN STS AssumeRole succeeds without an MFA token code (MFA serial is not configured or token code is empty), THE MFA_Tracker SHALL NOT update the last successful MFA timestamp in the State_File, preserving any previously recorded MFA timestamp
3. IF the State_File cannot be written due to a filesystem error, THEN THE CLI SHALL log a warning message to stderr indicating the write failure and continue the refresh operation without terminating
4. THE MFA_Tracker SHALL store the MFA timestamp in the existing `state.json` file alongside the token expiry field
5. IF the State_File does not yet exist when a write is required, THEN THE MFA_Tracker SHALL create the State_File with directory permissions 0700 and file permissions 0600

### Requirement 2: MFA Cooldown Evaluation

**User Story:** As a developer, I want the tool to skip MFA if I authenticated recently, so that I am not prompted for a code I already provided within the last hour.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth refresh` and the configured MFA serial is non-empty, THE MFA_Tracker SHALL read the last successful MFA timestamp from the State_File
2. WHEN the elapsed time since the last successful MFA authentication is less than the configured `mfa_cooldown_minutes` (converted to a duration), THE CLI SHALL skip the MFA step and attempt to assume the role without MFA parameters (no SerialNumber or TokenCode in the STS request)
3. WHEN the elapsed time since the last successful MFA authentication is greater than or equal to the configured `mfa_cooldown_minutes`, THE CLI SHALL proceed with MFA authentication as normal (TOTP from 1Password or manual prompt)
4. IF no last successful MFA timestamp exists in the State_File, THEN THE CLI SHALL proceed with MFA authentication as normal
5. IF the last successful MFA timestamp in the State_File is not a valid RFC3339 value, THEN THE CLI SHALL treat it as absent and proceed with MFA authentication as normal
6. WHEN MFA is skipped due to the cooldown, THE CLI SHALL display a message indicating that MFA was skipped because a recent authentication is still valid, including the time remaining in the cooldown window in the format `Xh Ym`
7. IF the configured MFA serial is empty, THEN THE CLI SHALL skip MFA evaluation entirely and assume the role without MFA parameters regardless of cooldown state

### Requirement 3: MFA Cooldown Configuration

**User Story:** As a developer, I want to configure how long the MFA cooldown lasts, so that I can balance convenience against my security requirements.

#### Acceptance Criteria

1. THE Config_Manager SHALL support an optional `mfa_cooldown_minutes` field in the configuration file with a default value of 60 when the field is absent or omitted
2. WHEN the user runs `claude-auth setup` and has provided a non-empty MFA device ARN, THE CLI SHALL prompt for the MFA cooldown duration (in minutes) with a default of 60
3. IF the user provides a value of 0 for `mfa_cooldown_minutes`, THEN THE Config_Manager SHALL store the value 0 and the system SHALL require MFA on every credential refresh regardless of elapsed time
4. IF the user provides a negative value for `mfa_cooldown_minutes`, THEN THE CLI SHALL reject the value with an error message indicating the cooldown must be zero or a positive integer, and SHALL not persist the configuration change
5. THE Config_Manager SHALL accept any positive integer value for `mfa_cooldown_minutes` without imposing an upper bound
6. IF the user provides a non-integer value (such as a decimal, empty string, or non-numeric text) for `mfa_cooldown_minutes` during setup, THEN THE CLI SHALL reject the value with an error message indicating a whole number is required, and SHALL re-prompt the user

### Requirement 4: Force MFA Override

**User Story:** As a developer, I want to force a fresh MFA authentication even within the cooldown window, so that I can re-authenticate when I suspect my session is invalid.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth refresh --force-mfa`, THE CLI SHALL bypass the MFA cooldown evaluation and proceed with MFA authentication (TOTP from 1Password or manual prompt) regardless of the last successful MFA timestamp in the State_File
2. WHEN `--force-mfa` is used and MFA authentication succeeds, THE MFA_Tracker SHALL record the current UTC timestamp (RFC3339 format) as the last successful MFA time in the State_File
3. THE CLI SHALL accept `--force-mfa` as a boolean long flag on the `refresh` command with a default value of false
4. WHEN `--force-mfa` is used and the cooldown is bypassed, THE CLI SHALL display a message indicating that MFA is being forced despite an active cooldown window
5. IF `--force-mfa` is used and the configured MFA serial is empty, THEN THE CLI SHALL return an error indicating that `--force-mfa` requires MFA to be configured in the profile

### Requirement 5: MFA Status Visibility

**User Story:** As a developer, I want to see when my MFA cooldown expires, so that I know whether my next refresh will require MFA.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth status` and a last successful MFA timestamp exists in the State_File and the elapsed time since that timestamp is less than the configured `mfa_cooldown_minutes`, THE CLI SHALL display the time remaining in the MFA cooldown window in the format `Xh Ym` where hours and minutes are truncated to whole minutes (not rounded up)
2. IF the user runs `claude-auth status` and the last successful MFA timestamp exists but the elapsed time since that timestamp is greater than or equal to the configured `mfa_cooldown_minutes`, THEN THE CLI SHALL display a message indicating that the next refresh will require MFA
3. IF the user runs `claude-auth status` and no last successful MFA timestamp exists in the State_File, THEN THE CLI SHALL display a message indicating that MFA has not been used yet
4. WHEN the user runs `claude-auth status` and `mfa_cooldown_minutes` is configured to 0, THE CLI SHALL display a message indicating that MFA rate-limiting is disabled and omit cooldown timing information
5. IF the user runs `claude-auth status` and the last successful MFA timestamp in the State_File is not a valid RFC3339 value, THEN THE CLI SHALL treat it as if no MFA timestamp exists and display a message indicating that MFA has not been used yet
6. IF the user runs `claude-auth status` and the configured `mfa_serial` is empty, THEN THE CLI SHALL omit MFA cooldown information entirely

### Requirement 6: Graceful Fallback on MFA-less Assume Failure

**User Story:** As a developer, I want the tool to fall back to MFA authentication if assuming the role without MFA fails, so that I am not left with a broken refresh when the role requires MFA.

#### Acceptance Criteria

1. IF the Role_Assumer attempts AssumeRole without MFA (due to cooldown skip) and STS returns an AccessDenied error, THEN THE CLI SHALL display a message indicating that MFA-less assumption failed and MFA is being requested, perform MFA authentication (TOTP from 1Password or manual prompt), and retry the AssumeRole call exactly once with MFA parameters
2. WHEN the fallback MFA authentication succeeds, THE MFA_Tracker SHALL update the last successful MFA timestamp in the State_File
3. IF the fallback MFA authentication also fails, THEN THE CLI SHALL return the error from the second (MFA) attempt without further retries
4. IF the Role_Assumer attempts AssumeRole without MFA (due to cooldown skip) and STS returns an error other than AccessDenied (e.g., network failure, expired credentials, or malformed request), THEN THE CLI SHALL return that error immediately without attempting MFA fallback
5. IF the Role_Assumer attempts AssumeRole with MFA (not a cooldown-skipped attempt) and STS returns an error, THEN THE CLI SHALL return that error immediately without triggering the fallback path

### Requirement 7: Clear MFA State

**User Story:** As a developer, I want `claude-auth clear` to also remove the MFA timestamp, so that a full reset forces MFA on the next refresh.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth clear`, THE CLI SHALL remove the State_File (containing the last successful MFA timestamp and the token expiry) and the credential env file from the configuration directory, leaving the configuration file (config.json) intact
2. IF the State_File does not exist when `claude-auth clear` is run, THEN THE CLI SHALL complete without error and display a message indicating nothing was cleared
3. WHEN the State_File has been removed by `claude-auth clear`, THE CLI SHALL require MFA on the next `claude-auth refresh` invocation regardless of how recently MFA was last completed
4. WHEN `claude-auth clear` successfully removes one or more files, THE CLI SHALL display a confirmation message indicating the stored token and MFA state have been cleared
