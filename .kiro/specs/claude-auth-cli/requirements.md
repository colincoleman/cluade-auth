# Requirements Document

## Introduction

claude-auth is a Go CLI that enables developers to use Claude Platform on AWS by managing the authentication flow: fetching long-term IAM credentials from 1Password, assuming an IAM role with MFA to get short-term credentials, and generating a presigned bearer token (ANTHROPIC_AWS_API_KEY) that Claude Code accepts. The token is injected into child processes on demand, so launching Claude normally still uses the personal account and AWS platform usage is opt-in per invocation.

## Glossary

- **CLI**: The claude-auth command-line interface application
- **1Password_Client**: The integration layer that communicates with the 1Password desktop application via its SDK to read and write credentials, gated by biometric authentication (Touch ID)
- **Token_Generator**: The component that creates presigned SigV4 bearer tokens for the `aws-external-anthropic` service
- **Config_Manager**: The component responsible for loading, saving, and validating configuration from `~/.config/claude-auth/config.json`
- **Credential_Store**: The local file system storage at `~/.config/claude-auth/` holding `anthropic.env` (the token) and `state.json` (expiry metadata)
- **Role_Assumer**: The component that calls AWS STS AssumeRole with long-term credentials and MFA to obtain short-term session credentials
- **ANTHROPIC_AWS_API_KEY**: A self-contained presigned SigV4 bearer token that Claude Code uses for inference; scoped to a region and time-limited
- **Session_Credentials**: Short-term AWS credentials (access key, secret, session token) returned by STS AssumeRole
- **TOTP**: Time-based One-Time Password used for MFA authentication

## Requirements

### Requirement 1: Interactive Configuration Setup

**User Story:** As a developer, I want an interactive setup wizard, so that I can configure claude-auth with my AWS and 1Password details without manually editing JSON files.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth setup`, THE CLI SHALL prompt for the following fields in order: 1Password account name, vault name, item name, IAM role ARN, MFA device ARN, workspace region, workspace ID, and session duration (hours)
2. WHEN the user presses Enter without typing a value, THE CLI SHALL use the displayed default value for that field
3. IF the user provides an empty value for a required field (1Password account, role ARN, workspace ID) that has no default, THEN THE CLI SHALL exit with a non-zero status and display an error message indicating which field is required
4. WHEN all prompts are completed successfully, THE Config_Manager SHALL write the configuration to `~/.config/claude-auth/config.json` with file permissions 0600
5. IF the configuration directory `~/.config/claude-auth/` does not exist, THEN THE CLI SHALL create it with permissions 0700 before writing the configuration file
6. IF a configuration file already exists at `~/.config/claude-auth/config.json`, THEN THE CLI SHALL overwrite it with the new values provided during setup
7. WHEN setup completes successfully, THE CLI SHALL display next steps that reference the `store` command and the `refresh` command by name
8. IF the configuration file cannot be written due to a filesystem error, THEN THE CLI SHALL exit with a non-zero status and display an error message indicating the failure reason

### Requirement 2: Credential Storage in 1Password

**User Story:** As a developer, I want to store my long-term IAM credentials in 1Password, so that they are never written to disk in plaintext.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth store`, THE CLI SHALL prompt for the AWS Access Key ID and Secret Access Key
2. WHEN the Secret Access Key is entered on an interactive terminal, THE CLI SHALL suppress character echo during input
3. IF the user provides an empty value for either the Access Key ID or Secret Access Key, THEN THE CLI SHALL return an error indicating the field is required without contacting 1Password
4. WHEN non-empty credentials are provided, THE 1Password_Client SHALL create or update an item in the configured vault with the `access_key_id` and `secret_access_key` fields
5. IF the configured vault does not exist, THEN THE 1Password_Client SHALL create the vault before storing the item
6. IF no configuration file exists when `store` is run, THEN THE CLI SHALL run the setup wizard first before proceeding
7. IF the 1Password_Client fails to connect (application not running or biometric authentication denied), THEN THE CLI SHALL return an error indicating the connection failure reason
8. WHEN credentials are stored successfully, THE CLI SHALL display a confirmation message identifying the vault and item name
9. IF the configured MFA serial is non-empty, THEN THE CLI SHALL display a reminder to add a TOTP field to the 1Password item

### Requirement 3: Credential Refresh and Token Generation

**User Story:** As a developer, I want to refresh my credentials with a single command, so that I get a valid 12-hour Anthropic API key token for my session.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth refresh`, THE 1Password_Client SHALL fetch the long-term IAM credentials (access key ID and secret access key) from the configured vault and item
2. WHEN the 1Password item contains a TOTP field, THE Role_Assumer SHALL use the live TOTP code as the MFA token code automatically without prompting the user
3. IF the 1Password item lacks a TOTP field and the configured MFA serial is non-empty, THEN THE CLI SHALL prompt the user to enter a 6-digit MFA code manually
4. WHEN credentials and MFA are available, THE Role_Assumer SHALL call STS AssumeRole directly (single-step, not role chaining) with the configured role ARN, MFA serial, token code, region, and session duration (specified in whole hours, between 1 and 12 inclusive)
5. WHEN the configured MFA serial is empty, THE Role_Assumer SHALL call STS AssumeRole without MFA parameters
6. WHEN role assumption succeeds, THE Token_Generator SHALL create a presigned SigV4 bearer token scoped to the configured region with the configured session duration
7. WHEN the token is generated, THE Credential_Store SHALL write it to `~/.config/claude-auth/anthropic.env` with file permissions 0600
8. WHEN the token is generated, THE Credential_Store SHALL write the token expiry timestamp (RFC3339) to `~/.config/claude-auth/state.json`
9. IF the 1Password_Client fails to connect or the configured item is not found, THEN THE CLI SHALL return an error indicating the failure reason and directing the user to verify 1Password is unlocked and the item exists
10. IF STS AssumeRole fails, THEN THE CLI SHALL return an error indicating the STS failure reason

### Requirement 4: Command Execution with Anthropic Token

**User Story:** As a developer, I want to launch Claude Code (or any command) with the AWS platform token injected, so that I can use Claude Platform on AWS without it affecting my normal Claude usage.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth exec` with no additional command specified, THE CLI SHALL launch `claude` as the default command
2. WHEN the user runs `claude-auth exec -- <command> [args...]`, THE CLI SHALL launch the specified command with the provided arguments
3. WHEN launching a command, THE CLI SHALL inherit the current process environment, then read the stored ANTHROPIC_AWS_API_KEY from `anthropic.env` and inject the following environment variables (overriding any existing values): CLAUDE_CODE_USE_ANTHROPIC_AWS=1, AWS_REGION (set to the configured workspace region), ANTHROPIC_AWS_WORKSPACE_ID (set to the configured workspace ID), and ANTHROPIC_AWS_API_KEY
4. THE CLI SHALL replace the current process (via syscall.Exec) so that signals propagate correctly and interactive TUIs work properly
5. THE CLI SHALL NOT inject raw AWS credentials (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN) into the exec environment
6. IF no stored token is found in `anthropic.env`, THEN THE CLI SHALL return an error directing the user to run `claude-auth refresh`
7. IF the specified command binary cannot be found in PATH, THEN THE CLI SHALL return an error identifying the missing command
8. IF the configuration file cannot be loaded, THEN THE CLI SHALL return an error directing the user to run `claude-auth setup`

### Requirement 5: Token Status Display

**User Story:** As a developer, I want to see how much time remains on my current token, so that I know when I need to refresh.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth status`, THE CLI SHALL display the workspace region from configuration
2. IF no configuration file exists when `claude-auth status` is run, THEN THE CLI SHALL return an error directing the user to run `claude-auth setup`
3. WHEN a token expiry is recorded in state.json, THE CLI SHALL display the remaining hours and minutes until expiry (rounded down to whole minutes), and the absolute expiry time formatted in the user's local timezone
4. IF the token has expired (remaining time is zero or negative), THEN THE CLI SHALL display "EXPIRED" with the expiry timestamp in the user's local timezone
5. IF no token expiry is recorded in state.json, THEN THE CLI SHALL display a message directing the user to run `claude-auth refresh`
6. IF the token expiry value in state.json is not a valid RFC3339 timestamp, THEN THE CLI SHALL display the expiry as unknown and direct the user to run `claude-auth refresh`

### Requirement 6: Token Health Check

**User Story:** As a developer, I want to verify my setup is correct without making network calls, so that I can diagnose configuration and token issues locally.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth check`, THE CLI SHALL verify that the configuration file loads successfully and display the config file path
2. IF the configuration file fails to load, THEN THE CLI SHALL display an error indicator with the failure reason and exit
3. WHEN the configuration loads, THE CLI SHALL display the workspace ID and region
4. WHEN a token exists in `anthropic.env`, THE Token_Generator SHALL decode the token locally (no network call) to extract the embedded region and expiry
5. IF the token cannot be decoded, THEN THE CLI SHALL display a decode error indicator and continue without crashing
6. WHEN the token's embedded region matches the configured workspace region, THE CLI SHALL display a success indicator
7. IF the token's embedded region does not match the configured workspace region, THEN THE CLI SHALL display a mismatch warning identifying both regions and direct the user to run `claude-auth refresh`
8. WHEN the token expiry is determined, THE CLI SHALL display the remaining time with appropriate severity: success indicator for more than 30 minutes remaining, warning indicator for 30 minutes or less remaining, error indicator for expired tokens
9. IF the token expiry cannot be determined from the decoded token, THEN THE CLI SHALL display a warning indicating the expiry is unknown
10. IF no token exists in `anthropic.env`, THEN THE CLI SHALL display a message directing the user to run `claude-auth refresh`

### Requirement 7: Token Clearing

**User Story:** As a developer, I want to remove the stored token, so that I can force a fresh refresh or clean up for hygiene.

#### Acceptance Criteria

1. WHEN the user runs `claude-auth clear`, THE CLI SHALL attempt to delete both `anthropic.env` and `state.json` from the `~/.config/claude-auth` directory
2. IF neither `anthropic.env` nor `state.json` exists in the configuration directory, THEN THE CLI SHALL display a message indicating nothing to clear and exit with code 0
3. WHEN at least one file is successfully removed, THE CLI SHALL display a message confirming the stored token has been cleared and exit with code 0
4. IF file deletion fails for a reason other than the file not existing (e.g., permission denied), THEN THE CLI SHALL return a non-zero exit code and display an error message indicating which file could not be removed
5. WHEN only one of the two files exists, THE CLI SHALL delete the existing file without treating the missing file as an error

### Requirement 8: Token Generation Algorithm

**User Story:** As a developer, I want the token generation to produce tokens compatible with the Anthropic AWS external service, so that Claude Code accepts them as valid bearer tokens.

#### Acceptance Criteria

1. THE Token_Generator SHALL create tokens by presigning a POST request with an empty body to `https://aws-external-anthropic.amazonaws.com/` with the query parameter `Action=CallWithBearerToken`
2. THE Token_Generator SHALL use AWS SigV4 signing with the service name `aws-external-anthropic` and the configured region
3. THE Token_Generator SHALL set the `X-Amz-Expires` query parameter to the session duration expressed as a whole number of seconds
4. THE Token_Generator SHALL encode the presigned URL (minus the `https://` prefix, with `&Version=1` appended) as standard base64 (RFC 4648 alphabet with `=` padding) and prepend the prefix `aws-external-anthropic-api-key-`
5. THE Token_Generator SHALL produce tokens where decoding (stripping the `aws-external-anthropic-api-key-` prefix, base64-decoding, removing the trailing `&Version=1`) yields the original presigned URL without the `https://` scheme prefix
6. IF the AWS SigV4 presigning operation fails, THEN THE Token_Generator SHALL return an error indicating the signing failure

### Requirement 9: Token Decoding

**User Story:** As a developer, I want to decode tokens locally to inspect their region and expiry, so that diagnostic commands work without network access.

#### Acceptance Criteria

1. WHEN a token with the prefix `aws-external-anthropic-api-key-` is provided, THE Token_Generator SHALL decode it by stripping the prefix, base64-decoding, and parsing the embedded SigV4 query parameters
2. WHEN the decoded query parameters contain an `X-Amz-Credential` value, THE Token_Generator SHALL extract the region from the third slash-delimited segment of that value
3. WHEN the decoded query parameters contain both `X-Amz-Date` (in `YYYYMMDDTHHMMSSZ` format) and `X-Amz-Expires` (integer seconds), THE Token_Generator SHALL compute the expiry by adding `X-Amz-Expires` seconds to the `X-Amz-Date` timestamp
4. IF the token does not have the expected prefix `aws-external-anthropic-api-key-`, THEN THE Token_Generator SHALL return an error indicating an invalid token
5. IF the token payload is not valid base64, THEN THE Token_Generator SHALL return an error indicating invalid encoding
6. IF the decoded payload cannot be parsed as query parameters, THEN THE Token_Generator SHALL return an error indicating a malformed token payload
7. IF the `X-Amz-Credential` parameter is missing or contains fewer than 3 slash-delimited segments, THEN THE Token_Generator SHALL return an error indicating the region could not be extracted
8. IF the `X-Amz-Date` parameter is missing or not in `YYYYMMDDTHHMMSSZ` format, THEN THE Token_Generator SHALL return a zero expiry value indicating the expiry could not be determined

### Requirement 10: MFA Flexibility

**User Story:** As a developer, I want the tool to support both automatic TOTP from 1Password and manual MFA code entry, so that users who prefer not to store MFA seeds in 1Password can still use the tool.

#### Acceptance Criteria

1. WHEN the 1Password item contains a one-time-password (TOTP) field, THE 1Password_Client SHALL read the current code and pass it to the Role_Assumer without prompting the user
2. WHEN the 1Password item does not contain a TOTP field and the configured MFA serial is non-empty, THE CLI SHALL prompt the user to enter an MFA code, displaying the MFA serial ARN in the prompt
3. IF the user provides an empty MFA code when prompted, THEN THE CLI SHALL return an error indicating that an MFA code is required
4. IF the user provides an MFA code that is not exactly 6 numeric digits, THEN THE CLI SHALL return an error indicating the expected format
5. WHEN the MFA serial is empty (role does not require MFA), THE Role_Assumer SHALL call AssumeRole without MFA parameters
6. IF STS AssumeRole fails due to an invalid or expired MFA code, THEN THE CLI SHALL return an error indicating that the MFA code was rejected

### Requirement 11: Personal Account Isolation

**User Story:** As a developer, I want launching Claude normally (without claude-auth) to always use my personal Claude account, so that the AWS platform integration never interferes with my default workflow.

#### Acceptance Criteria

1. THE CLI SHALL NOT modify shell profile files (~/.zshrc, ~/.bashrc, ~/.bash_profile, ~/.profile, ~/.zprofile), shell rc files, or any persistent environment variable configuration outside its own config directory (`~/.config/claude-auth/`)
2. WHEN launching a child process via `claude-auth exec`, THE CLI SHALL inject AWS platform environment variables (CLAUDE_CODE_USE_ANTHROPIC_AWS, AWS_REGION, ANTHROPIC_AWS_WORKSPACE_ID, ANTHROPIC_AWS_API_KEY) only into that child process's environment, not into the parent shell or any other process
3. THE Credential_Store SHALL store the token exclusively in `~/.config/claude-auth/anthropic.env`, a path that is not in Claude Code's automatic configuration search paths, ensuring the token is only active when explicitly read and injected by `claude-auth exec`
4. THE CLI SHALL NOT install shell aliases, PATH modifications, or login hooks that would cause AWS platform variables to be present in shells launched without `claude-auth exec`
