package providers

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// ServiceProvider implements service management
type ServiceProvider struct {
	platform *PlatformChecker
}

// ServiceState represents the current state of a service
type ServiceState struct {
	Running bool
	Enabled bool
}

// NewServiceProvider creates a new service provider
func NewServiceProvider() *ServiceProvider {
	return &ServiceProvider{
		platform: &PlatformChecker{},
	}
}

// Validate validates service resource attributes
func (p *ServiceProvider) Validate(ctx context.Context, attributes map[string]interface{}) error {
	// Check for required attributes
	name, ok := attributes["name"]
	if !ok {
		return fmt.Errorf("service resource requires 'name' attribute")
	}

	// Validate name is a string
	_, ok = name.(string)
	if !ok {
		return fmt.Errorf("service 'name' must be a string")
	}

	// Validate state if present
	if state, hasState := attributes["state"].(string); hasState {
		if state != "running" && state != "stopped" && state != "restarted" && state != "reloaded" {
			return fmt.Errorf("service 'state' must be one of: running, stopped, restarted, reloaded")
		}
	}

	// Validate enabled if present
	if enabled, hasEnabled := attributes["enabled"]; hasEnabled {
		_, ok = enabled.(bool)
		if !ok {
			return fmt.Errorf("service 'enabled' must be a boolean")
		}
	}

	// Validate provider if present
	if provider, hasProvider := attributes["provider"].(string); hasProvider {
		initSystem := p.platform.DetectInitSystem()
		if provider != initSystem && provider != "auto" {
			// If provider is specified, warn but don't fail
			fmt.Printf("Warning: specified service provider '%s' differs from detected init system '%s'\n", provider, initSystem)
		}
	}

	return nil
}

// getServiceProvider returns the appropriate service provider
func (p *ServiceProvider) getServiceProvider(attributes map[string]interface{}) string {
	// Check if provider is explicitly specified
	if provider, hasProvider := attributes["provider"].(string); hasProvider && provider != "auto" {
		return provider
	}

	// Auto-detect provider
	return p.platform.DetectInitSystem()
}

// getServiceState gets the current running and enabled state of a service
func (p *ServiceProvider) getServiceState(provider, name string) (ServiceState, error) {
	state := ServiceState{
		Running: false,
		Enabled: false,
	}

	switch provider {
	case "systemd":
		// Check if service is running
		cmdStatus := exec.Command("systemctl", "is-active", name+".service")
		if err := cmdStatus.Run(); err == nil {
			state.Running = true
		}

		// Check if service is enabled
		cmdEnabled := exec.Command("systemctl", "is-enabled", name+".service")
		if err := cmdEnabled.Run(); err == nil {
			state.Enabled = true
		}

	case "upstart":
		// Check if service is running
		cmdStatus := exec.Command("status", name)
		output, err := cmdStatus.CombinedOutput()
		if err == nil && strings.Contains(string(output), "start/running") {
			state.Running = true
		}

		// Check if service is enabled (upstart uses .conf files in /etc/init)
		if _, err := os.Stat("/etc/init/" + name + ".conf"); err == nil {
			state.Enabled = true
		}

	case "sysvinit":
		// Check if service is running
		cmdStatus := exec.Command("service", name, "status")
		if err := cmdStatus.Run(); err == nil {
			state.Running = true
		}

		// Check if service is enabled (look for appropriate runlevel symlinks)
		for _, level := range []string{"2", "3", "4", "5"} {
			linkPath := "/etc/rc" + level + ".d/S*" + name
			matches, _ := filepath.Glob(linkPath)
			if len(matches) > 0 {
				state.Enabled = true
				break
			}
		}

	case "launchd":
		// Check if service is loaded
		cmdStatus := exec.Command("launchctl", "list")
		output, err := cmdStatus.CombinedOutput()
		if err == nil && strings.Contains(string(output), name) {
			state.Running = true
		}

		// Check if service is enabled (has a plist in the LaunchDaemons directory)
		plistPaths := []string{
			"/Library/LaunchDaemons/" + name + ".plist",
			"/Library/LaunchAgents/" + name + ".plist",
			"/System/Library/LaunchDaemons/" + name + ".plist",
			"/System/Library/LaunchAgents/" + name + ".plist",
		}

		for _, path := range plistPaths {
			if _, err := os.Stat(path); err == nil {
				state.Enabled = true
				break
			}
		}

	case "windows":
		// Check if service is running
		cmdStatus := exec.Command("sc", "query", name)
		output, err := cmdStatus.CombinedOutput()
		if err == nil && strings.Contains(string(output), "RUNNING") {
			state.Running = true
		}

		// Check if service is enabled
		cmdConfig := exec.Command("sc", "qc", name)
		configOutput, err := cmdConfig.CombinedOutput()
		if err == nil && strings.Contains(string(configOutput), "AUTO_START") {
			state.Enabled = true
		}
	}

	return state, nil
}

// Plan determines what changes would be made to a service
func (p *ServiceProvider) Plan(ctx context.Context, current, desired map[string]interface{}) (*ResourceState, error) {
	name := desired["name"].(string)

	// Get desired state
	desiredState := ""
	if state, hasState := desired["state"].(string); hasState {
		desiredState = state
	}

	// Get desired enabled state
	desiredEnabled := false
	if enabled, hasEnabled := desired["enabled"].(bool); hasEnabled {
		desiredEnabled = enabled
	}

	result := &ResourceState{
		Type:       "service",
		Name:       name,
		Attributes: desired,
		Status:     "unchanged",
	}

	// Get service provider
	provider := p.getServiceProvider(desired)

	// Get current service state
	currentState, err := p.getServiceState(provider, name)
	if err != nil {
		return nil, err
	}

	// Check if changes are needed
	needsChange := false

	if desiredState == "running" && !currentState.Running {
		needsChange = true
	} else if desiredState == "stopped" && currentState.Running {
		needsChange = true
	} else if desiredState == "restarted" || desiredState == "reloaded" {
		needsChange = true
	}

	if desiredEnabled != currentState.Enabled {
		needsChange = true
	}

	if needsChange {
		result.Status = "planned"
	}

	return result, nil
}

// Apply applies the desired state to a service
func (p *ServiceProvider) Apply(ctx context.Context, state *ResourceState) (*ResourceState, error) {
	name := state.Attributes["name"].(string)

	// Get desired state
	desiredState := ""
	if state, hasState := state.Attributes["state"].(string); hasState {
		desiredState = state
	}

	// Get desired enabled state
	desiredEnabled := false
	if enabled, hasEnabled := state.Attributes["enabled"].(bool); hasEnabled {
		desiredEnabled = enabled
	}

	result := &ResourceState{
		Type:       state.Type,
		Name:       state.Name,
		Attributes: state.Attributes,
		Status:     "unchanged",
	}

	// Get service provider
	provider := p.getServiceProvider(state.Attributes)

	// Get current service state
	currentState, err := p.getServiceState(provider, name)
	if err != nil {
		result.Status = "failed"
		result.Error = err
		return result, err
	}

	// Apply changes
	if desiredState != "" {
		switch desiredState {
		case "running":
			if !currentState.Running {
				if err := p.startService(provider, name); err != nil {
					result.Status = "failed"
					result.Error = err
					return result, err
				}
				result.Status = "updated"
			}
		case "stopped":
			if currentState.Running {
				if err := p.stopService(provider, name); err != nil {
					result.Status = "failed"
					result.Error = err
					return result, err
				}
				result.Status = "updated"
			}
		case "restarted":
			if err := p.restartService(provider, name); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "updated"
		case "reloaded":
			if err := p.reloadService(provider, name); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "updated"
		}
	}

	// Set service enabled/disabled state
	if desiredEnabled != currentState.Enabled {
		if desiredEnabled {
			if err := p.enableService(provider, name); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
		} else {
			if err := p.disableService(provider, name); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
		}

		if result.Status == "unchanged" {
			result.Status = "updated"
		}
	}

	return result, nil
}

// startService starts a service
func (p *ServiceProvider) startService(provider, name string) error {
	var cmd *exec.Cmd

	switch provider {
	case "systemd":
		cmd = exec.Command("systemctl", "start", name+".service")
	case "upstart":
		cmd = exec.Command("start", name)
	case "sysvinit":
		cmd = exec.Command("service", name, "start")
	case "launchd":
		// Check if the service is already loaded
		loadState, _ := p.getServiceState(provider, name)
		if !loadState.Enabled {
			// Try to find the plist
			plistPaths := []string{
				"/Library/LaunchDaemons/" + name + ".plist",
				"/Library/LaunchAgents/" + name + ".plist",
			}

			plistPath := ""
			for _, path := range plistPaths {
				if _, err := os.Stat(path); err == nil {
					plistPath = path
					break
				}
			}

			if plistPath == "" {
				return fmt.Errorf("could not find plist for service %s", name)
			}

			// Load the service
			cmd = exec.Command("launchctl", "load", plistPath)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to load service %s: %v", name, err)
			}
		}

		// Start the service
		cmd = exec.Command("launchctl", "start", name)
	case "windows":
		cmd = exec.Command("sc", "start", name)
	default:
		return fmt.Errorf("unsupported service provider: %s", provider)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// stopService stops a service
func (p *ServiceProvider) stopService(provider, name string) error {
	var cmd *exec.Cmd

	switch provider {
	case "systemd":
		cmd = exec.Command("systemctl", "stop", name+".service")
	case "upstart":
		cmd = exec.Command("stop", name)
	case "sysvinit":
		cmd = exec.Command("service", name, "stop")
	case "launchd":
		cmd = exec.Command("launchctl", "stop", name)
	case "windows":
		cmd = exec.Command("sc", "stop", name)
	default:
		return fmt.Errorf("unsupported service provider: %s", provider)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// restartService restarts a service
func (p *ServiceProvider) restartService(provider, name string) error {
	var cmd *exec.Cmd

	switch provider {
	case "systemd":
		cmd = exec.Command("systemctl", "restart", name+".service")
	case "upstart":
		cmd = exec.Command("restart", name)
	case "sysvinit":
		cmd = exec.Command("service", name, "restart")
	case "launchd":
		// For launchd, we need to stop and then start the service
		if err := p.stopService(provider, name); err != nil {
			return err
		}
		return p.startService(provider, name)
	case "windows":
		// For Windows, we need to stop and then start the service
		if err := p.stopService(provider, name); err != nil {
			return err
		}
		return p.startService(provider, name)
	default:
		return fmt.Errorf("unsupported service provider: %s", provider)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart service %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// reloadService reloads a service configuration
func (p *ServiceProvider) reloadService(provider, name string) error {
	var cmd *exec.Cmd

	switch provider {
	case "systemd":
		cmd = exec.Command("systemctl", "reload", name+".service")
	case "upstart":
		cmd = exec.Command("reload", name)
	case "sysvinit":
		cmd = exec.Command("service", name, "reload")
	case "launchd":
		// For launchd, we need to unload and then load the service
		// First find the plist
		plistPaths := []string{
			"/Library/LaunchDaemons/" + name + ".plist",
			"/Library/LaunchAgents/" + name + ".plist",
		}

		plistPath := ""
		for _, path := range plistPaths {
			if _, err := os.Stat(path); err == nil {
				plistPath = path
				break
			}
		}

		if plistPath == "" {
			return fmt.Errorf("could not find plist for service %s", name)
		}

		// Unload the service
		unloadCmd := exec.Command("launchctl", "unload", plistPath)
		if err := unloadCmd.Run(); err != nil {
			return fmt.Errorf("failed to unload service %s: %v", name, err)
		}

		// Load the service
		loadCmd := exec.Command("launchctl", "load", plistPath)
		if err := loadCmd.Run(); err != nil {
			return fmt.Errorf("failed to load service %s: %v", name, err)
		}

		return nil
	case "windows":
		// Windows doesn't have a direct equivalent of reload
		return p.restartService(provider, name)
	default:
		return fmt.Errorf("unsupported service provider: %s", provider)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload service %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// enableService enables a service to start at boot
func (p *ServiceProvider) enableService(provider, name string) error {
	var cmd *exec.Cmd

	switch provider {
	case "systemd":
		cmd = exec.Command("systemctl", "enable", name+".service")
	case "upstart":
		// Upstart services are enabled by default when installed
		// Check if the .conf file exists
		if _, err := os.Stat("/etc/init/" + name + ".conf"); err != nil {
			return fmt.Errorf("upstart service %s not found", name)
		}
		return nil
	case "sysvinit":
		// Use update-rc.d to enable the service
		cmd = exec.Command("update-rc.d", name, "defaults")
	case "launchd":
		// Find the plist
		plistPaths := []string{
			"/Library/LaunchDaemons/" + name + ".plist",
			"/Library/LaunchAgents/" + name + ".plist",
		}

		plistPath := ""
		for _, path := range plistPaths {
			if _, err := os.Stat(path); err == nil {
				plistPath = path
				break
			}
		}

		if plistPath == "" {
			return fmt.Errorf("could not find plist for service %s", name)
		}

		// Load the service with the -w flag to enable it at boot
		cmd = exec.Command("launchctl", "load", "-w", plistPath)
	case "windows":
		cmd = exec.Command("sc", "config", name, "start=auto")
	default:
		return fmt.Errorf("unsupported service provider: %s", provider)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable service %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// disableService disables a service from starting at boot
func (p *ServiceProvider) disableService(provider, name string) error {
	var cmd *exec.Cmd

	switch provider {
	case "systemd":
		cmd = exec.Command("systemctl", "disable", name+".service")
	case "upstart":
		// Create an override file to disable the service
		overridePath := "/etc/init/" + name + ".override"
		err := ioutil.WriteFile(overridePath, []byte("manual"), 0644)
		if err != nil {
			return fmt.Errorf("failed to create upstart override file: %v", err)
		}
		return nil
	case "sysvinit":
		// Use update-rc.d to disable the service
		cmd = exec.Command("update-rc.d", name, "disable")
	case "launchd":
		// Find the plist
		plistPaths := []string{
			"/Library/LaunchDaemons/" + name + ".plist",
			"/Library/LaunchAgents/" + name + ".plist",
		}

		plistPath := ""
		for _, path := range plistPaths {
			if _, err := os.Stat(path); err == nil {
				plistPath = path
				break
			}
		}

		if plistPath == "" {
			return fmt.Errorf("could not find plist for service %s", name)
		}

		// Unload the service with the -w flag to disable it at boot
		cmd = exec.Command("launchctl", "unload", "-w", plistPath)
	case "windows":
		cmd = exec.Command("sc", "config", name, "start=demand")
	default:
		return fmt.Errorf("unsupported service provider: %s", provider)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable service %s: %v\nOutput: %s", name, err, string(output))
	}

	return nil
}

// CreateLaunchdPlist creates a launchd plist file for a service
func (p *ServiceProvider) CreateLaunchdPlist(name, command string, runAtBoot bool, keepAlive bool) error {
	// Only applicable on Darwin
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("CreateLaunchdPlist is only applicable on macOS")
	}

	// Define the plist template
	const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{ .Label }}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{ .Command }}</string>
    </array>
    <key>RunAtLoad</key>
    <{{ .RunAtLoad }}/>
    {{ if .KeepAlive }}
    <key>KeepAlive</key>
    <true/>
    {{ end }}
</dict>
</plist>`

	// Parse the template
	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %v", err)
	}

	// Define the template data
	data := struct {
		Label     string
		Command   string
		RunAtLoad string
		KeepAlive bool
	}{
		Label:     name,
		Command:   command,
		RunAtLoad: "true",
		KeepAlive: keepAlive,
	}

	if !runAtBoot {
		data.RunAtLoad = "false"
	}

	// Create the plist file
	plistPath := "/Library/LaunchDaemons/" + name + ".plist"
	file, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %v", err)
	}
	defer file.Close()

	// Execute the template
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute plist template: %v", err)
	}

	// Set the permissions
	if err := os.Chmod(plistPath, 0644); err != nil {
		return fmt.Errorf("failed to set plist file permissions: %v", err)
	}

	// Change ownership to root:wheel
	if err := exec.Command("sudo", "chown", "root:wheel", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to set plist file ownership: %v", err)
	}

	return nil
}

// CreateSystemdService creates a systemd service file
func (p *ServiceProvider) CreateSystemdService(name, description, command string, wantedBy string) error {
	// Only applicable on Linux with systemd
	if runtime.GOOS != "linux" || p.platform.DetectInitSystem() != "systemd" {
		return fmt.Errorf("CreateSystemdService is only applicable on Linux with systemd")
	}

	// Define the service file template
	const serviceTemplate = `[Unit]
Description={{ .Description }}

[Service]
ExecStart={{ .Command }}
Restart=on-failure
RestartSec=5

[Install]
WantedBy={{ .WantedBy }}
`

	// Parse the template
	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse service template: %v", err)
	}

	// Define the template data
	data := struct {
		Description string
		Command     string
		WantedBy    string
	}{
		Description: description,
		Command:     command,
		WantedBy:    wantedBy,
	}

	// Create the service file
	servicePath := "/etc/systemd/system/" + name + ".service"
	file, err := os.Create(servicePath)
	if err != nil {
		return fmt.Errorf("failed to create service file: %v", err)
	}
	defer file.Close()

	// Execute the template
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute service template: %v", err)
	}

	// Set the permissions
	if err := os.Chmod(servicePath, 0644); err != nil {
		return fmt.Errorf("failed to set service file permissions: %v", err)
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %v", err)
	}

	return nil
}

// CreateUpstartService creates an upstart service file
func (p *ServiceProvider) CreateUpstartService(name, description, command string, runLevels []string) error {
	// Only applicable on Linux with upstart
	if runtime.GOOS != "linux" || p.platform.DetectInitSystem() != "upstart" {
		return fmt.Errorf("CreateUpstartService is only applicable on Linux with upstart")
	}

	// Define the service file template
	const serviceTemplate = `# {{ .Name }} - {{ .Description }}
#
# This service is managed by goconfig

description "{{ .Description }}"

start on {{ .StartOn }}
stop on runlevel [!{{ .RunLevels }}]

respawn
respawn limit 10 5

exec {{ .Command }}
`

	// Parse the template
	tmpl, err := template.New("service").Parse(serviceTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse service template: %v", err)
	}

	// Join runlevels
	runLevelStr := strings.Join(runLevels, "")

	// Define the template data
	data := struct {
		Name        string
		Description string
		Command     string
		StartOn     string
		RunLevels   string
	}{
		Name:        name,
		Description: description,
		Command:     command,
		StartOn:     "runlevel [" + runLevelStr + "]",
		RunLevels:   runLevelStr,
	}

	// Create the service file
	servicePath := "/etc/init/" + name + ".conf"
	file, err := os.Create(servicePath)
	if err != nil {
		return fmt.Errorf("failed to create service file: %v", err)
	}
	defer file.Close()

	// Execute the template
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute service template: %v", err)
	}

	// Set the permissions
	if err := os.Chmod(servicePath, 0644); err != nil {
		return fmt.Errorf("failed to set service file permissions: %v", err)
	}

	return nil
}

// CreateWindowsService creates a Windows service
func (p *ServiceProvider) CreateWindowsService(name, displayName, description, command string, startType string) error {
	// Only applicable on Windows
	if runtime.GOOS != "windows" {
		return fmt.Errorf("CreateWindowsService is only applicable on Windows")
	}

	// Validate start type
	validStartTypes := map[string]string{
		"auto":         "auto",
		"automatic":    "auto",
		"manual":       "demand",
		"demand":       "demand",
		"disabled":     "disabled",
		"delayed-auto": "delayed-auto",
	}

	startTypeValue, ok := validStartTypes[strings.ToLower(startType)]
	if !ok {
		return fmt.Errorf("invalid start type '%s', must be one of: auto, manual, disabled, delayed-auto", startType)
	}

	// Create the service
	createCmd := exec.Command("sc", "create", name,
		fmt.Sprintf(`binPath=%s`, command),
		fmt.Sprintf(`DisplayName=%s`, displayName),
		fmt.Sprintf(`start=%s`, startTypeValue),
	)

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create service: %v", err)
	}

	// Set the description
	descCmd := exec.Command("sc", "description", name, description)
	if err := descCmd.Run(); err != nil {
		return fmt.Errorf("failed to set service description: %v", err)
	}

	return nil
}
