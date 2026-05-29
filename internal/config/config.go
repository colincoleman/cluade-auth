package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir  = ".config/claude-auth"
	configFile = "config.json"
	stateFile  = "state.json"
	envFile    = "anthropic.env"
)

type Config struct {
	OnePasswordAccount string `json:"onepassword_account"`
	Vault              string `json:"vault"`
	Item               string `json:"item"`
	// RoleARN is the IAM role to assume. It must hold the
	// aws-external-anthropic:CreateInference permission.
	RoleARN string `json:"role_arn"`
	// MFASerial is the ARN of the MFA device required to assume RoleARN.
	// Leave empty if the role does not require MFA.
	MFASerial string `json:"mfa_serial,omitempty"`
	// WorkspaceRegion is the region where the Claude Platform workspace was
	// provisioned. Claude Code uses it to build the API endpoint.
	WorkspaceRegion string `json:"workspace_region"`
	WorkspaceID     string `json:"workspace_id"`
	SessionDuration int    `json:"session_duration_hours"`
}

type State struct {
	AnthropicTokenExpiry string `json:"anthropic_token_expiry"`
}

func DefaultConfig() Config {
	return Config{
		Vault:           "Developer",
		Item:            "AWS IAM - Claude",
		RoleARN:         "arn:aws:iam::917709839926:role/rootRole",
		MFASerial:       "arn:aws:iam::917709839926:mfa/colin.coleman.localadmin",
		WorkspaceRegion: "eu-west-1",
		SessionDuration: 12,
	}
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFile), nil
}

func StatePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

func EnvPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, envFile), nil
}

func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found — run 'claude-auth setup' first")
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, configFile)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func Exists() bool {
	path, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func LoadState() (*State, error) {
	path, err := StatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return &State{}, nil
	}
	return &s, nil
}

func SaveState(s *State) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, stateFile)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
