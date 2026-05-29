# claude-auth

A small CLI that loads short-lived credentials for **Claude Platform on AWS** into your
session, on demand, with your long-term AWS credentials kept in **1Password**.

It mints a short-lived `ANTHROPIC_AWS_API_KEY` token (the same format the AWS Console
generates) and launches Claude Code — or any command — with that token set, so your
personal Claude account stays the default and the AWS platform is opt-in per invocation.

```
claude-auth refresh            # once per session (Touch ID + MFA) → 12h token
claude-auth exec -- claude     # run Claude Code against Claude Platform on AWS
```

---

## How it works

```
refresh:
  1Password item  ──▶  access key + secret  (Touch ID)
        │
        ├──▶  MFA code  (from a 1Password TOTP field, or typed at the prompt)
        ▼
  STS AssumeRole(<role>, SerialNumber=<mfa>, TokenCode=<code>)   ← one step, with MFA
        │            (the role holds aws-external-anthropic:CreateInference)
        ▼
  sign a presigned SigV4 token  ──▶  ANTHROPIC_AWS_API_KEY  ──▶  ~/.config/claude-auth/anthropic.env

exec:
  export CLAUDE_CODE_USE_ANTHROPIC_AWS=1, AWS_REGION, ANTHROPIC_AWS_WORKSPACE_ID,
         ANTHROPIC_AWS_API_KEY     then run your command (no raw AWS creds in the env)
```

Key design points:

- The `ANTHROPIC_AWS_API_KEY` is a **self-contained bearer token** — once set, Claude Code
  ignores AWS credentials entirely, so no raw AWS keys are left in your shell.
- The token is only valid because it's **signed by a role that has the
  `aws-external-anthropic:CreateInference` permission**. We assume that role **directly in one
  step** (an "initial assumption from user credentials"), which is *not* role chaining and so
  can last up to the role's `MaxSessionDuration` (12h). The two-step
  `GetSessionToken → AssumeRole` pattern is role chaining and is hard-capped at 1h by AWS.
- Nothing is written to `~/.aws/credentials`. Long-term keys live only in 1Password.

---

## Prerequisites

- **Go 1.21+** (to build) — `brew install go`
- **1Password desktop app**, signed in, with **Settings → Developer → "Integrate with other
  apps"** enabled (this allows the SDK to read secrets via Touch ID).
- **AWS CLI** — `brew install awscli` (only needed for the one-time role setup below).
- An **AWS account subscribed to Claude Platform on AWS**, with a **workspace** (note its
  workspace ID and region — find them in the AWS Console under *Claude Platform on AWS →
  Workspaces*).
- An **IAM user** with:
  - a virtual **MFA device** (TOTP),
  - long-term access keys,
  - permission to assume the role below.

---

## 1. Build & install

```bash
git clone <this-repo> claude-auth && cd claude-auth
go build -o /usr/local/bin/claude-auth .
```

## 2. Create the signing role (one-time, AWS side)

The token must be signed by a role that holds `aws-external-anthropic:CreateInference`. Use
the AWS-managed **`AnthropicLimitedAccess`** policy (the narrowest managed policy that includes
inference). Create a dedicated role with a **12-hour** max session and a trust policy that lets
your IAM user assume it with MFA.

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
  --max-session-duration 43200            # 12 hours — required for 12h tokens

aws iam attach-role-policy --role-name claude-platform \
  --policy-arn arn:aws:iam::aws:policy/AnthropicLimitedAccess
```

> If you can't run the AWS CLI yet (no base profile), you can create the role in the AWS
> Console instead, or — once `claude-auth` is configured against an admin role — via
> `claude-auth aws-exec -- aws iam create-role ...` (see [aws-exec](#aws-exec) below).

## 3. Store the IAM keys in 1Password

```bash
claude-auth setup     # interactive config wizard (see below)
claude-auth store     # prompts for the AWS Access Key ID + Secret, saves them to 1Password
```

If the role requires MFA, add the MFA **one-time password (TOTP)** to the *same* 1Password item:
open the item in the 1Password app and add a "one-time password" field using your MFA device's
setup secret. `claude-auth` will then read the live code automatically. (If you skip this,
`refresh` simply prompts you to type the 6-digit code.)

## 4. Configure (`claude-auth setup`)

The wizard writes `~/.config/claude-auth/config.json`:

| Prompt | Example |
|--------|---------|
| 1Password account name | `you@example.com` |
| 1Password vault | `Developer` |
| 1Password item name | `AWS IAM - Claude` |
| IAM role ARN to assume | `arn:aws:iam::<ACCOUNT_ID>:role/claude-platform` |
| MFA device ARN (blank if none) | `arn:aws:iam::<ACCOUNT_ID>:mfa/<IAM_USER>` |
| Workspace region | `eu-west-1` |
| Anthropic workspace ID | `wrkspc_…` |

> The defaults baked into the build are specific to the original author's account — change them
> for your own `<ACCOUNT_ID>`, role, MFA serial, region, and workspace.

---

## Daily usage

```bash
claude-auth refresh            # once per session: Touch ID + MFA → writes a 12h token
claude-auth status             # show time remaining on the current token
claude-auth check              # verify config + token region/expiry (no network call)

claude-auth exec -- claude     # run Claude Code against Claude Platform on AWS
claude-auth exec -- $SHELL     # an AWS-platform subshell (exit to return)
```

Optional convenience alias in `~/.zshrc` (nothing else is needed there):

```zsh
alias claude-aws='claude-auth exec -- claude'
```

### aws-exec

Run any command with **short-term AWS credentials** from the assumed role exported as
`AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` (nothing persisted to disk):

```bash
claude-auth aws-exec -- aws sts get-caller-identity
claude-auth aws-exec -- aws iam update-role --role-name claude-platform --max-session-duration 43200
```

---

## Command reference

| Command | What it does |
|---------|--------------|
| `setup` | Interactive config wizard → `~/.config/claude-auth/config.json` |
| `store` | Prompt for IAM access key + secret, save to 1Password |
| `refresh` | 1Password creds + MFA → assume role → sign + write `ANTHROPIC_AWS_API_KEY` |
| `status` | Show token expiry |
| `check` | Decode the token locally; verify region matches + not expired |
| `exec -- <cmd>` | Run `<cmd>` with the Anthropic token + workspace vars injected |
| `aws-exec -- <cmd>` | Run `<cmd>` with short-term AWS creds from the assumed role |

---

## Troubleshooting

**`403 … not authorized to perform: aws-external-anthropic:CreateInference`**
The role you're assuming lacks the inference permission. Attach `AnthropicLimitedAccess`
(or `AnthropicFullAccess`) to it. Confirm `role_arn` in your config points at that role.

**Token only lasts 1 hour**
The role's `MaxSessionDuration` is 1h (the `create-role` default). Raise it:
`claude-auth aws-exec -- aws iam update-role --role-name <role> --max-session-duration 43200`.
Also ensure you're on a recent build — older logic used the two-step
`GetSessionToken → AssumeRole` flow, which AWS hard-caps at 1h (role chaining).

**`invalid MFA one time pass code`**
Use a fresh code from the device registered as your `mfa_serial` (codes are single-use and
rotate every 30s). Verify the serial with
`aws iam list-mfa-devices --user-name <IAM_USER>`.

**`Please run /login` inside Claude Code, or it uses your personal account**
`/login` does not apply to Claude Platform on AWS. Make sure you launched via
`claude-auth exec -- claude` so `CLAUDE_CODE_USE_ANTHROPIC_AWS=1` and the token are set.

**Region mismatch / `check` shows the wrong region**
`workspace_region` must match the region your workspace was provisioned in (it's in the
workspace ARN). Fix it in the config and run `claude-auth refresh`.

---

## Configuration file

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
  "session_duration_hours": 12
}
```

Secrets written by `refresh` live in `~/.config/claude-auth/anthropic.env` (mode `0600`); the
token's expiry is tracked in `~/.config/claude-auth/state.json`.
