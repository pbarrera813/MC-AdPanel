package minecraft

import (
	"context"
	"testing"
)

func TestValidatePluginUpdateURLRejectsPrivateIP(t *testing.T) {
	allowed := map[string]struct{}{
		"127.0.0.1": {},
	}
	if _, err := validatePluginUpdateURL(context.Background(), "https://127.0.0.1/plugin.jar", allowed); err == nil {
		t.Fatalf("expected localhost update URL to be rejected")
	}
}

func TestValidatePluginUpdateURLAllowsAllowlistedPublicIP(t *testing.T) {
	allowed := map[string]struct{}{
		"93.184.216.34": {}, // example.com
	}
	if _, err := validatePluginUpdateURL(context.Background(), "https://93.184.216.34/plugin.jar", allowed); err != nil {
		t.Fatalf("expected allowlisted public IP URL to pass validation: %v", err)
	}
}

func TestValidatePluginUpdateURLRejectsNonHTTPS(t *testing.T) {
	allowed := map[string]struct{}{
		"example.com": {},
	}
	if _, err := validatePluginUpdateURL(context.Background(), "http://example.com/plugin.jar", allowed); err == nil {
		t.Fatalf("expected non-https URL to be rejected")
	}
}

func TestHostAllowedByPolicySupportsSuffix(t *testing.T) {
	allowed := map[string]struct{}{
		"example.com": {},
	}
	if !hostAllowedByPolicy("downloads.example.com", allowed) {
		t.Fatalf("expected subdomain match to be allowed")
	}
}
