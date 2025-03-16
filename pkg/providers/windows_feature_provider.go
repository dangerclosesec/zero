package providers

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// WindowsFeatureProvider implements Windows feature management
type WindowsFeatureProvider struct {
	platform *PlatformChecker
}

// NewWindowsFeatureProvider creates a new Windows feature provider
func NewWindowsFeatureProvider() *WindowsFeatureProvider {
	return &WindowsFeatureProvider{
		platform: &PlatformChecker{},
	}
}

// Validate validates Windows feature resource attributes
func (p *WindowsFeatureProvider) Validate(ctx context.Context, attributes map[string]interface{}) error {
	// Only valid on Windows
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windows_feature provider is only valid on Windows")
	}

	// Check for required attributes
	name, ok := attributes["name"]
	if !ok {
		return fmt.Errorf("windows_feature resource requires 'name' attribute")
	}

	// Validate name is a string
	_, ok = name.(string)
	if !ok {
		return fmt.Errorf("windows_feature 'name' must be a string")
	}

	// Validate state if present
	if state, hasState := attributes["state"].(string); hasState {
		if state != "installed" && state != "removed" {
			return fmt.Errorf("windows_feature 'state' must be one of: installed, removed")
		}
	}

	// Check if DISM command is available
	if !p.isDismAvailable() && !p.isPowerShellAvailable() {
		return fmt.Errorf("neither DISM nor PowerShell (with Server Manager module) are available")
	}

	return nil
}

// isFeatureInstalled checks if a Windows feature is installed
func (p *WindowsFeatureProvider) isFeatureInstalled(name string) (bool, error) {
	// Prefer DISM if available, fallback to PowerShell
	if p.isDismAvailable() {
		return p.isFeatureInstalledDism(name)
	}
	return p.isFeatureInstalledPowerShell(name)
}

// isFeatureInstalledDism checks if a feature is installed using DISM
func (p *WindowsFeatureProvider) isFeatureInstalledDism(name string) (bool, error) {
	cmd := exec.Command("dism", "/Online", "/Get-FeatureInfo", fmt.Sprintf("/FeatureName:%s", name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("error checking feature with DISM: %v", err)
	}

	outputStr := string(output)
	return strings.Contains(outputStr, "State : Enabled"), nil
}

// isFeatureInstalledPowerShell checks if a feature is installed using PowerShell
func (p *WindowsFeatureProvider) isFeatureInstalledPowerShell(name string) (bool, error) {
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Get-WindowsFeature -Name %s | Select-Object -ExpandProperty Installed", name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("error checking feature with PowerShell: %v", err)
	}

	outputStr := strings.TrimSpace(string(output))
	return outputStr == "True", nil
}

// isDismAvailable checks if DISM is available
func (p *WindowsFeatureProvider) isDismAvailable() bool {
	_, err := exec.LookPath("dism")
	return err == nil
}

// isPowerShellAvailable checks if PowerShell with Server Manager module is available
func (p *WindowsFeatureProvider) isPowerShellAvailable() bool {
	// Check if PowerShell is available
	_, err := exec.LookPath("powershell")
	if err != nil {
		return false
	}

	// Check if the ServerManager module is available
	cmd := exec.Command("powershell", "-Command", "Get-Module -ListAvailable -Name ServerManager")
	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

// Plan determines what changes would be made to a Windows feature
func (p *WindowsFeatureProvider) Plan(ctx context.Context, current, desired map[string]interface{}) (*ResourceState, error) {
	// Only valid on Windows
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("windows_feature provider is only valid on Windows")
	}

	name := desired["name"].(string)

	// Get desired state or default to "installed"
	state := "installed"
	if desiredState, ok := desired["state"].(string); ok {
		state = desiredState
	}

	result := &ResourceState{
		Type:       "windows_feature",
		Name:       name,
		Attributes: desired,
		Status:     "planned",
	}

	// Check if the feature is installed
	installed, err := p.isFeatureInstalled(name)
	if err != nil {
		return nil, err
	}

	if state == "installed" && installed {
		// Feature is already installed
		result.Status = "unchanged"
	} else if state == "removed" && !installed {
		// Feature is already removed
		result.Status = "unchanged"
	}

	return result, nil
}

// Apply installs or removes a Windows feature
func (p *WindowsFeatureProvider) Apply(ctx context.Context, state *ResourceState) (*ResourceState, error) {
	// Only valid on Windows
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("windows_feature provider is only valid on Windows")
	}

	name := state.Attributes["name"].(string)

	// Get desired state or default to "installed"
	desiredState := "installed"
	if state, ok := state.Attributes["state"].(string); ok {
		desiredState = state
	}

	result := &ResourceState{
		Type:       state.Type,
		Name:       state.Name,
		Attributes: state.Attributes,
	}

	// Check current state
	installed, err := p.isFeatureInstalled(name)
	if err != nil {
		result.Status = "failed"
		result.Error = err
		return result, err
	}

	if desiredState == "installed" && !installed {
		// Install the feature
		if err := p.installFeature(name); err != nil {
			result.Status = "failed"
			result.Error = err
			return result, err
		}
		result.Status = "created"
	} else if desiredState == "removed" && installed {
		// Remove the feature
		if err := p.removeFeature(name); err != nil {
			result.Status = "failed"
			result.Error = err
			return result, err
		}
		result.Status = "deleted"
	} else {
		// No change needed
		result.Status = "unchanged"
	}

	return result, nil
}

// installFeature installs a Windows feature
func (p *WindowsFeatureProvider) installFeature(name string) error {
	// Prefer DISM if available, fallback to PowerShell
	if p.isDismAvailable() {
		return p.installFeatureDism(name)
	}
	return p.installFeaturePowerShell(name)
}

// installFeatureDism installs a feature using DISM
func (p *WindowsFeatureProvider) installFeatureDism(name string) error {
	cmd := exec.Command("dism", "/Online", "/Enable-Feature", fmt.Sprintf("/FeatureName:%s", name), "/All")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error installing feature with DISM: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// installFeaturePowerShell installs a feature using PowerShell
func (p *WindowsFeatureProvider) installFeaturePowerShell(name string) error {
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Install-WindowsFeature -Name %s", name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error installing feature with PowerShell: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// removeFeature removes a Windows feature
func (p *WindowsFeatureProvider) removeFeature(name string) error {
	// Prefer DISM if available, fallback to PowerShell
	if p.isDismAvailable() {
		return p.removeFeatureDism(name)
	}
	return p.removeFeaturePowerShell(name)
}

// removeFeatureDism removes a feature using DISM
func (p *WindowsFeatureProvider) removeFeatureDism(name string) error {
	cmd := exec.Command("dism", "/Online", "/Disable-Feature", fmt.Sprintf("/FeatureName:%s", name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error removing feature with DISM: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// removeFeaturePowerShell removes a feature using PowerShell
func (p *WindowsFeatureProvider) removeFeaturePowerShell(name string) error {
	cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Uninstall-WindowsFeature -Name %s", name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error removing feature with PowerShell: %v\nOutput: %s", err, string(output))
	}
	return nil
}
