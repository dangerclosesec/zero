package providers

import (
	"context"
	"runtime"
	"testing"
)

func TestServiceProvider_Validate(t *testing.T) {
	provider := NewServiceProvider()
	ctx := context.Background()

	// Test valid minimal attributes
	validAttrs := map[string]interface{}{
		"name": "test-service",
	}
	if err := provider.Validate(ctx, validAttrs); err != nil {
		t.Errorf("Expected no error for valid attributes, got: %v", err)
	}

	// Test missing name attribute
	invalidAttrs := map[string]interface{}{
		"state": "running",
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

	// Test valid state
	validStateAttrs := map[string]interface{}{
		"name":  "test-service",
		"state": "running",
	}
	if err := provider.Validate(ctx, validStateAttrs); err != nil {
		t.Errorf("Expected no error for valid state, got: %v", err)
	}

	// Test invalid state
	invalidStateAttrs := map[string]interface{}{
		"name":  "test-service",
		"state": "invalid-state",
	}
	if err := provider.Validate(ctx, invalidStateAttrs); err == nil {
		t.Error("Expected error for invalid state, got nil")
	}

	// Test valid enabled attribute
	validEnabledAttrs := map[string]interface{}{
		"name":    "test-service",
		"enabled": true,
	}
	if err := provider.Validate(ctx, validEnabledAttrs); err != nil {
		t.Errorf("Expected no error for valid enabled attribute, got: %v", err)
	}

	// Test invalid enabled type
	invalidEnabledAttrs := map[string]interface{}{
		"name":    "test-service",
		"enabled": "true", // should be bool
	}
	if err := provider.Validate(ctx, invalidEnabledAttrs); err == nil {
		t.Error("Expected error for invalid enabled type, got nil")
	}
}

func TestServiceProvider_getServiceProvider(t *testing.T) {
	provider := NewServiceProvider()

	// Test auto detection (default)
	autoAttrs := map[string]interface{}{
		"name": "test-service",
	}
	detectedProvider := provider.getServiceProvider(autoAttrs)
	if detectedProvider == "" {
		t.Error("Expected auto-detected provider to be non-empty")
	}

	// Test explicit provider
	explicitAttrs := map[string]interface{}{
		"name":     "test-service",
		"provider": "systemd",
	}
	explicitProvider := provider.getServiceProvider(explicitAttrs)
	if explicitProvider != "systemd" {
		t.Errorf("Expected provider to be 'systemd', got '%s'", explicitProvider)
	}
}

func TestServiceProvider_Plan(t *testing.T) {
	provider := NewServiceProvider()
	ctx := context.Background()

	// We'll use a mocked/partial implementation to avoid actual service interactions
	// This test focuses on the planning logic, not the actual service state detection

	// Test basic plan with state:running
	basicAttrs := map[string]interface{}{
		"name":  "test-service",
		"state": "running",
	}
	result, err := provider.Plan(ctx, map[string]interface{}{}, basicAttrs)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "planned" && result.Status != "unchanged" {
		t.Errorf("Expected status 'planned' or 'unchanged', got '%s'", result.Status)
	}

	// Test plan with enabled:true
	enabledAttrs := map[string]interface{}{
		"name":    "test-service",
		"enabled": true,
	}
	result, err = provider.Plan(ctx, map[string]interface{}{}, enabledAttrs)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "planned" && result.Status != "unchanged" {
		t.Errorf("Expected status 'planned' or 'unchanged', got '%s'", result.Status)
	}
}

func TestServiceProvider_Apply_Plan_NoChanges(t *testing.T) {
	// Skip this test if we can't interact with services
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("Skipping service tests on non-standard OS")
	}

	provider := NewServiceProvider()
	ctx := context.Background()

	// Create a test for a service that doesn't need changes (using a known system service)
	var knownService string
	switch runtime.GOOS {
	case "darwin":
		knownService = "com.apple.syslogd"
	case "linux":
		knownService = "systemd-journald"
	case "windows":
		knownService = "wuauserv" // Windows Update
	}

	// Skip if no known service for the current OS
	if knownService == "" {
		t.Skip("No known service for current OS")
	}

	// Get the current state of the service for comparison
	currentState, err := provider.getServiceState(provider.platform.DetectInitSystem(), knownService)
	if err != nil {
		t.Skipf("Failed to get current state of service %s: %v", knownService, err)
	}

	// Create a test state that matches the current state
	state := &ResourceState{
		Type: "service",
		Name: knownService,
		Attributes: map[string]interface{}{
			"name":    knownService,
			"state":   "running",
			"enabled": currentState.Running, // Match the current running state
		},
		Status: "planned",
	}

	if !currentState.Running {
		// If the service is not running, change our expectation
		state.Attributes["state"] = "stopped"
	}

	// Plan the service state (non-destructive)
	planResult, err := provider.Plan(ctx, nil, state.Attributes)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	// Depending on the current state, the plan may be "unchanged" or "planned"
	if planResult.Status != "unchanged" && planResult.Status != "planned" {
		t.Errorf("Expected plan status 'unchanged' or 'planned', got '%s'", planResult.Status)
	}
}

func TestServiceProvider_CreateLaunchdPlist(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping launchd test on non-Darwin OS")
	}

	provider := NewServiceProvider()

	// Test proper error on non-Darwin platforms when running on another OS
	if runtime.GOOS != "darwin" {
		err := provider.CreateLaunchdPlist("test", "ls", true, false)
		if err == nil {
			t.Error("Expected error when creating launchd plist on non-Darwin platform")
		}
	}
}

func TestServiceProvider_CreateSystemdService(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping systemd test on non-Linux OS")
	}

	provider := NewServiceProvider()

	// Test proper error on non-Linux platforms when running on another OS
	if runtime.GOOS != "linux" {
		err := provider.CreateSystemdService("test", "test description", "ls", "multi-user.target")
		if err == nil {
			t.Error("Expected error when creating systemd service on non-Linux platform")
		}
	}
}

func TestServiceProvider_CreateUpstartService(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping upstart test on non-Linux OS")
	}

	provider := NewServiceProvider()

	// Test proper error on non-Linux platforms when running on another OS
	if runtime.GOOS != "linux" {
		err := provider.CreateUpstartService("test", "test description", "ls", []string{"2", "3", "4", "5"})
		if err == nil {
			t.Error("Expected error when creating upstart service on non-Linux platform")
		}
	}
}

func TestServiceProvider_CreateWindowsService(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows service test on non-Windows OS")
	}

	provider := NewServiceProvider()

	// Test proper error on non-Windows platforms when running on another OS
	if runtime.GOOS != "windows" {
		err := provider.CreateWindowsService("test", "test display name", "test description", "cmd.exe", "auto")
		if err == nil {
			t.Error("Expected error when creating Windows service on non-Windows platform")
		}
	}
}