package onepw

import (
	"context"
	"fmt"

	"github.com/1password/onepassword-sdk-go"
)

const (
	fieldAccessKeyID     = "access_key_id"
	fieldSecretAccessKey = "secret_access_key"
)

type Client struct {
	op *onepassword.Client
}

func New(ctx context.Context, accountName string) (*Client, error) {
	op, err := onepassword.NewClient(ctx,
		onepassword.WithDesktopAppIntegration(accountName),
		onepassword.WithIntegrationInfo("claude-auth", "v1.0.0"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to 1Password: %w", err)
	}
	return &Client{op: op}, nil
}

func (c *Client) GetCredentials(ctx context.Context, vaultName, itemTitle string) (accessKeyID, secretAccessKey string, err error) {
	vaultID, err := c.findVaultID(ctx, vaultName)
	if err != nil {
		return "", "", err
	}
	item, err := c.findItem(ctx, vaultID, itemTitle)
	if err != nil {
		return "", "", err
	}
	for _, f := range item.Fields {
		switch f.Title {
		case fieldAccessKeyID:
			accessKeyID = f.Value
		case fieldSecretAccessKey:
			secretAccessKey = f.Value
		}
	}
	if accessKeyID == "" || secretAccessKey == "" {
		return "", "", fmt.Errorf("item %q in vault %q is missing required fields", itemTitle, vaultName)
	}
	return accessKeyID, secretAccessKey, nil
}

func (c *Client) StoreCredentials(ctx context.Context, vaultName, itemTitle, accessKeyID, secretAccessKey string) error {
	vaultID, err := c.findOrCreateVaultID(ctx, vaultName)
	if err != nil {
		return err
	}

	fields := []onepassword.ItemField{
		{ID: fieldAccessKeyID, Title: fieldAccessKeyID, FieldType: onepassword.ItemFieldTypeText, Value: accessKeyID},
		{ID: fieldSecretAccessKey, Title: fieldSecretAccessKey, FieldType: onepassword.ItemFieldTypeConcealed, Value: secretAccessKey},
	}

	existing, err := c.findItemOptional(ctx, vaultID, itemTitle)
	if err != nil {
		return err
	}

	if existing != nil {
		for i, f := range existing.Fields {
			switch f.Title {
			case fieldAccessKeyID:
				existing.Fields[i].Value = accessKeyID
			case fieldSecretAccessKey:
				existing.Fields[i].Value = secretAccessKey
			}
		}
		if _, err := c.op.Items().Put(ctx, *existing); err != nil {
			return fmt.Errorf("failed to update 1Password item: %w", err)
		}
		fmt.Printf("Updated existing item %q in vault %q\n", itemTitle, vaultName)
		return nil
	}

	_, err = c.op.Items().Create(ctx, onepassword.ItemCreateParams{
		Category: onepassword.ItemCategoryAPICredentials,
		VaultID:  vaultID,
		Title:    itemTitle,
		Fields:   fields,
	})
	if err != nil {
		return fmt.Errorf("failed to create 1Password item: %w", err)
	}
	fmt.Printf("Created item %q in vault %q\n", itemTitle, vaultName)
	return nil
}

func (c *Client) findVaultID(ctx context.Context, name string) (string, error) {
	id, found, err := c.lookupVault(ctx, name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("vault %q not found — run 'claude-auth store' to create it", name)
	}
	return id, nil
}

func (c *Client) findOrCreateVaultID(ctx context.Context, name string) (string, error) {
	id, found, err := c.lookupVault(ctx, name)
	if err != nil {
		return "", err
	}
	if found {
		return id, nil
	}
	vault, err := c.op.Vaults().Create(ctx, onepassword.VaultCreateParams{Title: name})
	if err != nil {
		return "", fmt.Errorf("failed to create vault %q: %w", name, err)
	}
	fmt.Printf("Created vault %q\n", name)
	return vault.ID, nil
}

func (c *Client) lookupVault(ctx context.Context, name string) (id string, found bool, err error) {
	vaults, err := c.op.Vaults().List(ctx)
	if err != nil {
		return "", false, fmt.Errorf("failed to list vaults: %w", err)
	}
	for _, v := range vaults {
		if v.Title == name {
			return v.ID, true, nil
		}
	}
	return "", false, nil
}

func (c *Client) findItem(ctx context.Context, vaultID, title string) (*onepassword.Item, error) {
	item, err := c.findItemOptional(ctx, vaultID, title)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("item %q not found — run 'claude-auth store' first", title)
	}
	return item, nil
}

func (c *Client) findItemOptional(ctx context.Context, vaultID, title string) (*onepassword.Item, error) {
	overviews, err := c.op.Items().List(ctx, vaultID)
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}
	for _, ov := range overviews {
		if ov.Title == title {
			item, err := c.op.Items().Get(ctx, vaultID, ov.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to get item: %w", err)
			}
			return &item, nil
		}
	}
	return nil, nil
}
