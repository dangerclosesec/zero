package providers

import (
	"context"
	"fmt"
	"os/exec"
)

// PackageProvider implements package management
type PackageProvider struct {
	platform *PlatformChecker
}

// NewPackageProvider creates a new package provider
func NewPackageProvider() *PackageProvider {
	return &PackageProvider{
		platform: &PlatformChecker{},
	}
}

// Validate validates package resource attributes
func (p *PackageProvider) Validate(ctx context.Context, attributes map[string]interface{}) error {
	// Check for required attributes
	name, ok := attributes["name"]
	if !ok {
		return fmt.Errorf("package resource requires 'name' attribute")
	}

	// Validate name is a string
	_, ok = name.(string)
	if !ok {
		return fmt.Errorf("package 'name' must be a string")
	}

	// Validate state if present
	if state, hasState := attributes["state"].(string); hasState {
		if state != "installed" && state != "removed" && state != "latest" {
			return fmt.Errorf("package 'state' must be one of: installed, removed, latest")
		}
	}

	// Check package manager availability
	pkgManager := p.platform.GetPackageManager()
	if pkgManager == "unknown" {
		return fmt.Errorf("no supported package manager found on this system")
	}

	return nil
}

// isPackageInstalled checks if a package is installed
func (p *PackageProvider) isPackageInstalled(name string) (bool, error) {
	pkgManager := p.platform.GetPackageManager()

	var cmd *exec.Cmd

	switch pkgManager {
	case "apt":
		cmd = exec.Command("dpkg", "-s", name)
	case "dnf", "yum":
		cmd = exec.Command(pkgManager, "list", "installed", name)
	case "pacman":
		cmd = exec.Command("pacman", "-Q", name)
	case "zypper":
		cmd = exec.Command("zypper", "search", "--installed-only", name)
	case "apk":
		cmd = exec.Command("apk", "info", "-e", name)
	case "brew":
		cmd = exec.Command("brew", "list", "--versions", name)
	case "port":
		cmd = exec.Command("port", "installed", name)
	case "choco":
		cmd = exec.Command("choco", "list", "--local-only", name)
	case "winget":
		cmd = exec.Command("winget", "list", "--exact", name)
	default:
		return false, fmt.Errorf("unsupported package manager: %s", pkgManager)
	}

	err := cmd.Run()
	return err == nil, nil
}

// getLatestVersion checks if a package has the latest version
func (p *PackageProvider) getLatestVersion(name string) (string, error) {
	pkgManager := p.platform.GetPackageManager()

	var cmd *exec.Cmd

	switch pkgManager {
	case "apt":
		cmd = exec.Command("apt-cache", "policy", name)
	case "dnf":
		cmd = exec.Command("dnf", "info", name)
	case "yum":
		cmd = exec.Command("yum", "info", name)
	case "pacman":
		cmd = exec.Command("pacman", "-Si", name)
	case "zypper":
		cmd = exec.Command("zypper", "info", name)
	case "apk":
		cmd = exec.Command("apk", "info", name)
	case "brew":
		cmd = exec.Command("brew", "info", "--json=v1", name)
	case "port":
		cmd = exec.Command("port", "info", name)
	case "choco":
		cmd = exec.Command("choco", "info", name, "--limit-output")
	case "winget":
		cmd = exec.Command("winget", "show", name)
	default:
		return "", fmt.Errorf("unsupported package manager: %s", pkgManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get package info: %v", err)
	}

	// For simplicity, we're just returning the output of the command
	// In a real implementation, we would parse the output to extract the version
	return string(output), nil
}

// Plan determines what changes would be made to a package
func (p *PackageProvider) Plan(ctx context.Context, current, desired map[string]interface{}) (*ResourceState, error) {
	name := desired["name"].(string)

	// Get desired state or default to "installed"
	state := "installed"
	if desiredState, ok := desired["state"].(string); ok {
		state = desiredState
	}

	result := &ResourceState{
		Type:       "package",
		Name:       name,
		Attributes: desired,
		Status:     "unchanged",
	}

	// Check if the package is installed
	installed, err := p.isPackageInstalled(name)
	if err != nil {
		return nil, err
	}

	switch state {
	case "installed":
		if !installed {
			result.Status = "planned"
		}
	case "removed":
		if installed {
			result.Status = "planned"
		}
	case "latest":
		if !installed {
			result.Status = "planned"
		} else {
			// Check if package is at the latest version
			// This is a simplified implementation
			result.Status = "planned" // Assume we always need to update
		}
	}

	return result, nil
}

// Apply installs, updates, or removes a package
func (p *PackageProvider) Apply(ctx context.Context, state *ResourceState) (*ResourceState, error) {
	name := state.Attributes["name"].(string)

	// Get desired state or default to "installed"
	desiredState := "installed"
	if state, ok := state.Attributes["state"].(string); ok {
		desiredState = state
	}

	// Get version if specified
	version := ""
	if v, ok := state.Attributes["version"].(string); ok {
		version = v
	}

	result := &ResourceState{
		Type:       state.Type,
		Name:       state.Name,
		Attributes: state.Attributes,
		Status:     "unchanged",
	}

	// Check if the package is installed
	installed, err := p.isPackageInstalled(name)
	if err != nil {
		result.Status = "failed"
		result.Error = err
		return result, err
	}

	pkgManager := p.platform.GetPackageManager()

	switch desiredState {
	case "installed":
		if !installed {
			if err := p.installPackage(pkgManager, name, version); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "created"
		}
	case "removed":
		if installed {
			if err := p.removePackage(pkgManager, name); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "deleted"
		}
	case "latest":
		if !installed {
			if err := p.installPackage(pkgManager, name, ""); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "created"
		} else {
			if err := p.updatePackage(pkgManager, name); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "updated"
		}
	}

	return result, nil
}

// installPackage installs a package
func (p *PackageProvider) installPackage(pkgManager, name, version string) error {
	var cmd *exec.Cmd

	// Prepare package name with version if specified
	pkg := name
	if version != "" {
		switch pkgManager {
		case "apt":
			pkg = fmt.Sprintf("%s=%s", name, version)
		case "dnf", "yum":
			pkg = fmt.Sprintf("%s-%s", name, version)
		case "pacman":
			pkg = fmt.Sprintf("%s=%s", name, version)
		case "zypper":
			pkg = fmt.Sprintf("%s=%s", name, version)
		case "apk":
			pkg = fmt.Sprintf("%s=%s", name, version)
		case "brew":
			// Homebrew doesn't support installing specific versions directly
			pkg = name
		case "port":
			pkg = fmt.Sprintf("%s@%s", name, version)
		case "choco":
			pkg = fmt.Sprintf("%s --version=%s", name, version)
		case "winget":
			pkg = fmt.Sprintf("%s --version %s", name, version)
		}
	}

	switch pkgManager {
	case "apt":
		cmd = exec.Command("apt-get", "install", "-y", pkg)
	case "dnf":
		cmd = exec.Command("dnf", "install", "-y", pkg)
	case "yum":
		cmd = exec.Command("yum", "install", "-y", pkg)
	case "pacman":
		cmd = exec.Command("pacman", "-S", "--noconfirm", pkg)
	case "zypper":
		cmd = exec.Command("zypper", "install", "-y", pkg)
	case "apk":
		cmd = exec.Command("apk", "add", pkg)
	case "brew":
		cmd = exec.Command("brew", "install", pkg)
	case "port":
		cmd = exec.Command("port", "install", pkg)
	case "choco":
		cmd = exec.Command("choco", "install", "--yes", pkg)
	case "winget":
		cmd = exec.Command("winget", "install", "--exact", "--silent", pkg)
	default:
		return fmt.Errorf("unsupported package manager: %s", pkgManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install package %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// removePackage removes a package
func (p *PackageProvider) removePackage(pkgManager, name string) error {
	var cmd *exec.Cmd

	switch pkgManager {
	case "apt":
		cmd = exec.Command("apt-get", "remove", "-y", name)
	case "dnf":
		cmd = exec.Command("dnf", "remove", "-y", name)
	case "yum":
		cmd = exec.Command("yum", "remove", "-y", name)
	case "pacman":
		cmd = exec.Command("pacman", "-R", "--noconfirm", name)
	case "zypper":
		cmd = exec.Command("zypper", "remove", "-y", name)
	case "apk":
		cmd = exec.Command("apk", "del", name)
	case "brew":
		cmd = exec.Command("brew", "uninstall", name)
	case "port":
		cmd = exec.Command("port", "uninstall", name)
	case "choco":
		cmd = exec.Command("choco", "uninstall", "--yes", name)
	case "winget":
		cmd = exec.Command("winget", "uninstall", "--exact", "--silent", name)
	default:
		return fmt.Errorf("unsupported package manager: %s", pkgManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove package %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// updatePackage updates a package to the latest version
func (p *PackageProvider) updatePackage(pkgManager, name string) error {
	var cmd *exec.Cmd

	switch pkgManager {
	case "apt":
		cmd = exec.Command("apt-get", "install", "--only-upgrade", "-y", name)
	case "dnf":
		cmd = exec.Command("dnf", "update", "-y", name)
	case "yum":
		cmd = exec.Command("yum", "update", "-y", name)
	case "pacman":
		cmd = exec.Command("pacman", "-Syu", "--noconfirm", name)
	case "zypper":
		cmd = exec.Command("zypper", "update", "-y", name)
	case "apk":
		cmd = exec.Command("apk", "upgrade", name)
	case "brew":
		cmd = exec.Command("brew", "upgrade", name)
	case "port":
		cmd = exec.Command("port", "upgrade", name)
	case "choco":
		cmd = exec.Command("choco", "upgrade", "--yes", name)
	case "winget":
		cmd = exec.Command("winget", "upgrade", "--exact", "--silent", name)
	default:
		return fmt.Errorf("unsupported package manager: %s", pkgManager)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update package %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}
