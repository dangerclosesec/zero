package providers

import (
	"context"
	"runtime"
	"testing"
)

func TestWindowsFeatureProvider_Validate(t *testing.T) {
	provider := NewWindowsFeatureProvider()
	ctx := context.Background()

	// Skip on non-Windows platforms with a meaningful test message
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows feature tests on non-Windows platform")
	}

	// Test valid minimal attributes
	validAttrs := map[string]interface{}{
		"name": "feature-name",
	}
	err := provider.Validate(ctx, validAttrs)
	// On non-Windows we expect an error about the provider being Windows-only
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		// On Windows, the validation might fail for other reasons like DISM not being available
		// or the feature name being invalid, but we'd need to run on a Windows system to test properly
		t.Logf("Validation result on Windows: %v", err)
	}

	// Test missing name attribute
	invalidAttrs := map[string]interface{}{
		"state": "installed",
	}
	if err := provider.Validate(ctx, invalidAttrs); err == nil {
		t.Error("Expected error for missing name attribute, got nil")
	}

	// Test invalid name type
	invalidNameAttrs := map[string]interface{}{
		"name": 123, // should be string
	}
	if err := provider.Validate(ctx, invalidNameAttrs); err == nil {
		t.Error("Expected error for invalid name type, got nil")
	}

	// Test invalid state
	invalidStateAttrs := map[string]interface{}{
		"name":  "feature-name",
		"state": "invalid-state",
	}
	if err := provider.Validate(ctx, invalidStateAttrs); err == nil {
		t.Error("Expected error for invalid state, got nil")
	}

	// Test valid state: "installed"
	validInstalledAttrs := map[string]interface{}{
		"name":  "feature-name",
		"state": "installed",
	}
	err = provider.Validate(ctx, validInstalledAttrs)
	// Again, behavior depends on the OS
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		t.Logf("Validation result for 'installed' state on Windows: %v", err)
	}
	
	// Test valid state: "removed"
	validRemovedAttrs := map[string]interface{}{
		"name":  "feature-name",
		"state": "removed",
	}
	err = provider.Validate(ctx, validRemovedAttrs)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		t.Logf("Validation result for 'removed' state on Windows: %v", err)
	}
}

func TestWindowsFeatureProvider_Plan(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows feature plan test on non-Windows platform")
	}

	provider := NewWindowsFeatureProvider()
	ctx := context.Background()

	// Test plan with default "installed" state
	attrs := map[string]interface{}{
		"name": "feature-name",
	}
	_, err := provider.Plan(ctx, map[string]interface{}{}, attrs)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		// On Windows, we would check results more thoroughly
		t.Logf("Plan result on Windows: %v", err)
	}

	// Test plan with explicit "installed" state
	installedAttrs := map[string]interface{}{
		"name":  "feature-name",
		"state": "installed",
	}
	_, err = provider.Plan(ctx, map[string]interface{}{}, installedAttrs)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		t.Logf("Plan result for 'installed' state on Windows: %v", err)
	}

	// Test plan with "removed" state
	removedAttrs := map[string]interface{}{
		"name":  "feature-name",
		"state": "removed",
	}
	_, err = provider.Plan(ctx, map[string]interface{}{}, removedAttrs)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		t.Logf("Plan result for 'removed' state on Windows: %v", err)
	}
}

func TestWindowsFeatureProvider_Apply(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows feature apply test on non-Windows platform")
	}

	provider := NewWindowsFeatureProvider()
	ctx := context.Background()

	// Create a test state for "installed" state
	installedState := &ResourceState{
		Type: "windows_feature",
		Name: "feature-name",
		Attributes: map[string]interface{}{
			"name":  "feature-name",
			"state": "installed",
		},
		Status: "planned",
	}

	// Apply the state (this would only work on Windows)
	_, err := provider.Apply(ctx, installedState)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		// On Windows, we would check results more thoroughly
		t.Logf("Apply result on Windows: %v", err)
	}

	// Create a test state for "removed" state
	removedState := &ResourceState{
		Type: "windows_feature",
		Name: "feature-name",
		Attributes: map[string]interface{}{
			"name":  "feature-name",
			"state": "removed",
		},
		Status: "planned",
	}

	// Apply the state (this would only work on Windows)
	_, err = provider.Apply(ctx, removedState)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only provider on non-Windows OS")
		}
	} else {
		t.Logf("Apply result for 'removed' state on Windows: %v", err)
	}
}

func TestWindowsFeatureProvider_isFeatureInstalled(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows feature check test on non-Windows platform")
	}

	provider := NewWindowsFeatureProvider()

	// Test with a feature that should exist on most Windows systems
	// Note: This is a best effort test, as feature availability varies
	knownFeature := "NetFx4-AdvSrvs"
	installed, err := provider.isFeatureInstalled(knownFeature)
	if runtime.GOOS != "windows" {
		if err == nil {
			t.Error("Expected error for Windows-only function on non-Windows OS")
		}
	} else {
		t.Logf("Feature '%s' installed: %v (err: %v)", knownFeature, installed, err)
	}
}

func TestWindowsFeatureProvider_DISM_PowerShell_Availability(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows command availability test on non-Windows platform")
	}

	provider := NewWindowsFeatureProvider()

	// Check if DISM is available
	dismAvailable := provider.isDismAvailable()
	t.Logf("DISM available: %v", dismAvailable)

	// Check if PowerShell is available
	powershellAvailable := provider.isPowerShellAvailable()
	t.Logf("PowerShell available: %v", powershellAvailable)

	// At least one of these should be available on a Windows system
	if runtime.GOOS == "windows" && !dismAvailable && !powershellAvailable {
		t.Error("Expected at least one of DISM or PowerShell to be available on Windows")
	}
}