package providers

import (
	"context"
	"fmt"
	"runtime"
	"testing"
)

// MockProvider implements ResourceProvider for testing
type MockProvider struct {
	ValidateResponse error
	PlanResponse     *ResourceState
	PlanError        error
	ApplyResponse    *ResourceState
	ApplyError       error
}

func (m *MockProvider) Validate(ctx context.Context, attributes map[string]interface{}) error {
	return m.ValidateResponse
}

func (m *MockProvider) Plan(ctx context.Context, current, desired map[string]interface{}) (*ResourceState, error) {
	return m.PlanResponse, m.PlanError
}

func (m *MockProvider) Apply(ctx context.Context, state *ResourceState) (*ResourceState, error) {
	return m.ApplyResponse, m.ApplyError
}

func TestProviderRegistry_Register(t *testing.T) {
	registry := NewProviderRegistry()
	mockProvider := &MockProvider{}

	// Register a provider
	registry.Register("test", mockProvider)

	// Verify it was registered correctly
	provider, err := registry.Get("test")
	if err != nil {
		t.Errorf("Failed to get registered provider: %v", err)
	}

	if provider != mockProvider {
		t.Error("Retrieved provider does not match registered provider")
	}
}

func TestProviderRegistry_Get_NonExistent(t *testing.T) {
	registry := NewProviderRegistry()

	// Try to get a non-existent provider
	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent provider, got nil")
	}
}

func TestPlatformChecker_IsSupported(t *testing.T) {
	checker := &PlatformChecker{}
	currentOS := runtime.GOOS

	// Test with current OS - should be supported
	if !checker.IsSupported([]string{currentOS}) {
		t.Errorf("Expected current OS %s to be supported", currentOS)
	}

	// Test with "unix" platform on Linux/Darwin
	if currentOS == "linux" || currentOS == "darwin" {
		if !checker.IsSupported([]string{"unix"}) {
			t.Error("Expected 'unix' platform to be supported on Linux/Darwin")
		}
	}

	// Test with non-matching platform
	if checker.IsSupported([]string{"invalid-platform"}) {
		t.Error("Expected 'invalid-platform' to not be supported")
	}

	// Test with multiple platforms including current one
	if !checker.IsSupported([]string{"invalid-platform", currentOS}) {
		t.Errorf("Expected platform list including %s to be supported", currentOS)
	}
}

func TestPlatformChecker_DetectInitSystem(t *testing.T) {
	checker := &PlatformChecker{}
	initSystem := checker.DetectInitSystem()

	// We can't test the exact value since it depends on the OS,
	// but we can at least verify it returns something
	if initSystem == "" {
		t.Error("Expected DetectInitSystem to return a non-empty string")
	}

	// For non-Linux systems, verify expected values
	if runtime.GOOS == "darwin" && initSystem != "launchd" {
		t.Errorf("Expected 'launchd' init system on macOS, got %s", initSystem)
	}

	if runtime.GOOS == "windows" && initSystem != "windows" {
		t.Errorf("Expected 'windows' init system on Windows, got %s", initSystem)
	}
}

func TestPlatformChecker_IsCommandAvailable(t *testing.T) {
	checker := &PlatformChecker{}

	// Test a command that should be available on all platforms
	if !checker.IsCommandAvailable("go") {
		t.Skip("'go' command not found, skipping test")
	}

	// Test a command that definitely doesn't exist
	if checker.IsCommandAvailable("this_command_definitely_does_not_exist_12345") {
		t.Error("Expected non-existent command to return false")
	}
}

func TestPlatformChecker_GetPackageManager(t *testing.T) {
	checker := &PlatformChecker{}
	packageManager := checker.GetPackageManager()

	// We can only verify that it returns something
	if packageManager == "" {
		t.Error("Expected GetPackageManager to return a non-empty string")
	}
}

// Test utility for validation error formatting
func TestValidationErrorFormatting(t *testing.T) {
	message := "test error message"
	err := fmt.Errorf("validation error: %s", message)
	
	if err == nil {
		t.Fatal("Expected error to be created")
	}

	if err.Error() != "validation error: test error message" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}