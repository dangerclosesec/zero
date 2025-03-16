package providers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ResourceState represents the state of a resource
type ResourceState struct {
	Type       string
	Name       string
	Attributes map[string]interface{}
	Status     string // "created", "updated", "deleted", "unchanged", "failed"
	Error      error
}

// ResourceProvider defines the interface for all resource providers
type ResourceProvider interface {
	// Validate checks if the resource attributes are valid
	Validate(ctx context.Context, attributes map[string]interface{}) error

	// Plan returns what changes would be made
	Plan(ctx context.Context, current, desired map[string]interface{}) (*ResourceState, error)

	// Apply applies the changes
	Apply(ctx context.Context, state *ResourceState) (*ResourceState, error)
}

// ProviderRegistry maintains a mapping of resource types to their providers
type ProviderRegistry struct {
	providers map[string]ResourceProvider
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ResourceProvider),
	}
}

// Register registers a provider for a resource type
func (r *ProviderRegistry) Register(resourceType string, provider ResourceProvider) {
	r.providers[resourceType] = provider
}

// Get returns the provider for a resource type
func (r *ProviderRegistry) Get(resourceType string) (ResourceProvider, error) {
	provider, exists := r.providers[resourceType]
	if !exists {
		return nil, fmt.Errorf("no provider registered for resource type %s", resourceType)
	}
	return provider, nil
}

// PlatformChecker provides OS detection functionality
type PlatformChecker struct{}

// IsSupported checks if the current platform is in the list of supported platforms
func (p *PlatformChecker) IsSupported(platforms []string) bool {
	currentOS := runtime.GOOS

	for _, platform := range platforms {
		switch platform {
		case "linux", "darwin", "windows":
			if currentOS == platform {
				return true
			}
		case "unix":
			if currentOS == "linux" || currentOS == "darwin" {
				return true
			}
		}
	}

	return false
}

// DetectInitSystem detects the init system used on Linux
func (p *PlatformChecker) DetectInitSystem() string {
	// Only applicable on Linux
	if runtime.GOOS != "linux" {
		if runtime.GOOS == "darwin" {
			return "launchd"
		}
		if runtime.GOOS == "windows" {
			return "windows"
		}
		return "unknown"
	}

	// Check for systemd
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return "systemd"
	}

	// Check for upstart
	if _, err := os.Stat("/sbin/initctl"); err == nil {
		cmd := exec.Command("/sbin/initctl", "--version")
		output, err := cmd.CombinedOutput()
		if err == nil && strings.Contains(string(output), "upstart") {
			return "upstart"
		}
	}

	// Check for SysV init (fallback)
	if _, err := os.Stat("/etc/init.d"); err == nil {
		return "sysvinit"
	}

	return "unknown"
}

// IsCommandAvailable checks if a command is available on the system
func (p *PlatformChecker) IsCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// GetPackageManager detects the package manager on the system
func (p *PlatformChecker) GetPackageManager() string {
	switch runtime.GOOS {
	case "darwin":
		// Check for Homebrew first
		if p.IsCommandAvailable("brew") {
			return "brew"
		}
		// Check for MacPorts
		if p.IsCommandAvailable("port") {
			return "port"
		}
		return "unknown"

	case "windows":
		// Check for Chocolatey
		if p.IsCommandAvailable("choco") {
			return "choco"
		}
		// Check for Windows Package Manager (WinGet)
		if p.IsCommandAvailable("winget") {
			return "winget"
		}
		return "unknown"

	case "linux":
		// Debian/Ubuntu based
		if p.IsCommandAvailable("apt") || p.IsCommandAvailable("apt-get") {
			return "apt"
		}
		// RHEL/CentOS/Fedora
		if p.IsCommandAvailable("dnf") {
			return "dnf"
		}
		if p.IsCommandAvailable("yum") {
			return "yum"
		}
		// Arch Linux
		if p.IsCommandAvailable("pacman") {
			return "pacman"
		}
		// SUSE
		if p.IsCommandAvailable("zypper") {
			return "zypper"
		}
		// Alpine
		if p.IsCommandAvailable("apk") {
			return "apk"
		}
		return "unknown"

	default:
		return "unknown"
	}
}
