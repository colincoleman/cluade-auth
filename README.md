# claude-auth

**Use Claude Code via the AWS-hosted Claude Platform — without touching your personal Claude account or leaving long-term credentials on disk.**

claude-auth bridges 1Password, AWS STS, and Claude Code so you can authenticate with a single command. Your IAM keys stay in 1Password, MFA is handled via Touch ID or a one-time code, and a short-lived bearer token is injected only when you need it.

```
claude-auth refresh   # 1Password + MFA → 12-hour token
claude-auth exec      # launch Claude Code in platform mode
```

Your normal `claude` command continues to use your personal account. The AWS platform is opt-in per invocation.

## Why?

Claude Platform on AWS requires a presigned SigV4 bearer token to authenticate. Generating that token requires assuming an IAM role with MFA. Without claude-auth, you'd need to manually juggle AWS credentials, MFA codes, STS calls, and environment variables every time you want to use the platform.

claude-auth automates the entire flow: credentials from 1Password → MFA (automatic TOTP or manual) → role assumption → token generation → injection into Claude Code.

---

## Quickstart

```bash
# 1. Install
git clone <this-repo> && cd claude-auth
go build -o /usr/local/bin/claude-auth .

# 2. Configure (interactive wizard)
claude-auth setup

# 3. Authenticate
claude-auth refresh

# 4. Use Claude Code on the platform
claude-auth exec
```

That's it. `exec` launches Claude Code with the platform token injected. Run `claude-auth status` to check how long your token is valid.

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.21+** | `brew install go` |
| **1Password desktop app** | Signed in, with *Settings → Developer → "Integrate with other apps"* enabled |
| **Claude Platform on AWS workspace** | Sign up at the [AWS Console service page](https://console.aws.amazon.com/claude-platform/) — a default workspace is provisioned automatically. Note the workspace ID and region (found under *Workspaces* in the console). See the [setup guide](https://docs.aws.amazon.com/claude-platform/latest/userguide/setup.html) for full steps. |
| **IAM user** with long-term access keys | With a virtual MFA device (TOTP) and permission to assume the signing role |

---

## Detailed Setup

### 1. Create the signing role (one-time)

The token must be signed by a role with `aws-external-anthropic:CreateInference`. Create a role with a 12-hour max session and a trust policy requiring MFA:

```bash
cat > /tmp/trust.json <<JSON
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "AWS": "arn:aws:iam::<ACCOUNT_ID>:user/<IAM_USER>" },
    "Action": "sts:AssumeRole",
    "Condition": { "Bool": { "aws:MultiFactorAuthPresent": "true" } }
  }]
}
JSON

aws iam create-role --role-name claude-platform \
  --assume-role-policy-document file:///tmp/trust.json \
  --max-session-duration 43200

aws iam attach-role-policy --role-name claude-platform \
  --policy-arn arn:aws:iam::aws:policy/AnthropicLimitedAccess
```

### 2. Store credentials in 1Password

```bash
claude-auth store   # prompts for Access Key ID + Secret, saves to 1Password
```

### 3. Configure claude-auth

```bash
claude-auth setup   # interactive wizard → ~/.config/claude-auth/config.json
```

The wizard asks for: 1Password account, vault, item name, role ARN, MFA serial, workspace region, workspace ID, and session duration.

### 4. Add TOTP to 1Password (optional but recommended)

Add a "one-time password" field to the same 1Password item using your MFA device's setup secret. This lets `refresh` read the MFA code automatically via Touch ID. If you skip this, you'll be prompted to type the 6-digit code each time.

---

## Usage

### Day-to-day

```bash
claude-auth refresh              # authenticate and mint a 12-hour token
claude-auth exec                 # launch Claude Code in AWS platform mode
claude-auth exec -- $SHELL       # open a subshell with the token set
claude-auth exec -- claude "hi"  # pass arguments through
```

### Monitoring

```bash
claude-auth status               # show time remaining on current token
claude-auth check                # verify config + token health (offline)
```

### Maintenance

```bash
claude-auth clear                # delete stored token and MFA state
claude-auth refresh --force-mfa  # force fresh MFA even within cooldown
```

### Running AWS commands

Run any command with short-term AWS credentials from the assumed role:

```bash
claude-auth aws-exec -- aws sts get-caller-identity
claude-auth aws-exec -- aws iam update-role --role-name claude-platform --max-session-duration 43200
```

---

## Commands

| Command | Description |
|---------|-------------|
| `setup` | Interactive config wizard |
| `store` | Save IAM credentials to 1Password |
| `refresh` | Fetch creds from 1Password, assume role with MFA, generate token |
| `status` | Display token expiry, MFA cooldown, and time remaining |
| `check` | Decode token locally, verify region match and expiry |
| `exec [-- cmd]` | Run a command (default: `claude`) with the Anthropic token injected |
| `aws-exec -- cmd` | Run a command with short-term AWS credentials exported |
| `clear` | Delete the stored token and MFA state |

---

## How it works

```
1Password  →  IAM credentials (Touch ID)
                    │
                    ├── MFA code (auto from TOTP field, or manual prompt)
                    ▼
            STS AssumeRole (direct, not role-chained → up to 12h)
                    │
                    ▼
            Presigned SigV4 token → ~/.config/claude-auth/anthropic.env

exec injects:  CLAUDE_CODE_USE_ANTHROPIC_AWS=1
               AWS_REGION, ANTHROPIC_AWS_WORKSPACE_ID, ANTHROPIC_AWS_API_KEY
               (no raw AWS credentials leave the process)
```

The token is self-contained — Claude Code doesn't need AWS credentials once it has the bearer token. Role assumption is done in a single step (not `GetSessionToken → AssumeRole`), avoiding the 1-hour cap that AWS imposes on role-chained sessions.

### MFA rate-limiting

claude-auth tracks when you last authenticated with MFA. If you run `refresh` within the cooldown window (default: 60 minutes) and your token is still valid, it skips the refresh entirely — no 1Password fetch, no MFA prompt. Configure the cooldown during `setup` or set `mfa_cooldown_minutes` in your config (0 disables it).

---

## Configuration

`~/.config/claude-auth/config.json`:

```json
{
  "onepassword_account": "you@example.com",
  "vault": "Developer",
  "item": "AWS IAM - Claude",
  "role_arn": "arn:aws:iam::<ACCOUNT_ID>:role/claude-platform",
  "mfa_serial": "arn:aws:iam::<ACCOUNT_ID>:mfa/<IAM_USER>",
  "workspace_region": "eu-west-1",
  "workspace_id": "wrkspc_…",
  "session_duration_hours": 12,
  "mfa_cooldown_minutes": 60
}
```

All files are stored in `~/.config/claude-auth/` with mode `0600`:

| File | Purpose |
|------|---------|
| `config.json` | Persistent configuration |
| `anthropic.env` | Bearer token (`ANTHROPIC_AWS_API_KEY=...`) |
| `state.json` | Token expiry and last MFA timestamp |

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `403 … not authorized to perform: aws-external-anthropic:CreateInference` | Attach `AnthropicLimitedAccess` policy to your role |
| Token only lasts 1 hour | Raise the role's `MaxSessionDuration` to 43200 (12h) |
| `invalid MFA one time pass code` | Wait for a fresh code (they rotate every 30s and are single-use per window) |
| Claude Code asks to `/login` or uses personal account | Launch via `claude-auth exec`, not bare `claude` |
| Region mismatch in `check` | Set `workspace_region` in config to match your workspace's provisioned region |

---

## License

[MIT](LICENSE)
