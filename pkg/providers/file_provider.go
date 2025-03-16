package providers

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

// FileProvider implements file resource management
type FileProvider struct {
	platform *PlatformChecker
}

// NewFileProvider creates a new file provider
func NewFileProvider() *FileProvider {
	return &FileProvider{
		platform: &PlatformChecker{},
	}
}

// Validate validates file resource attributes
func (p *FileProvider) Validate(ctx context.Context, attributes map[string]interface{}) error {
	// Check for required attributes
	path, ok := attributes["path"]
	if !ok {
		return fmt.Errorf("file resource requires 'path' attribute")
	}

	// Validate path is a string
	_, ok = path.(string)
	if !ok {
		return fmt.Errorf("file 'path' must be a string")
	}

	// Check for mutually exclusive attributes
	if content, hasContent := attributes["content"]; hasContent {
		if source, hasSource := attributes["source"]; hasSource && source != "" && content != "" {
			return fmt.Errorf("file resource cannot have both 'content' and 'source' attributes")
		}
	}

	// Validate state if present
	if state, hasState := attributes["state"]; hasState {
		stateStr, ok := state.(string)
		if !ok {
			return fmt.Errorf("file 'state' must be a string")
		}

		if stateStr != "present" && stateStr != "absent" && stateStr != "directory" {
			return fmt.Errorf("file 'state' must be one of: present, absent, directory")
		}
	}

	// Validate mode if present
	if mode, hasMode := attributes["mode"]; hasMode {
		modeStr, ok := mode.(string)
		if !ok {
			return fmt.Errorf("file 'mode' must be a string")
		}

		// Try to parse as octal
		_, err := strconv.ParseInt(modeStr, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid file mode: %s", modeStr)
		}
	}

	return nil
}

// fileExists checks if a file exists and is not a directory
func (p *FileProvider) fileExists(path string) (bool, os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, info, nil
}

// calculateMD5 calculates the MD5 hash of a file
func (p *FileProvider) calculateMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Plan determines what changes would be made to a file
func (p *FileProvider) Plan(ctx context.Context, current, desired map[string]interface{}) (*ResourceState, error) {
	path := desired["path"].(string)

	// Get desired state or default to "present"
	state := "present"
	if desiredState, ok := desired["state"].(string); ok {
		state = desiredState
	}

	result := &ResourceState{
		Type:       "file",
		Name:       path,
		Attributes: desired,
		Status:     "unchanged",
	}

	// Check if the file exists
	exists, fileInfo, err := p.fileExists(path)
	if err != nil {
		return nil, err
	}

	switch state {
	case "absent":
		if exists {
			// File exists, needs to be removed
			result.Status = "planned"
		}

	case "directory":
		if !exists {
			// Directory doesn't exist, needs to be created
			result.Status = "planned"
		} else if !fileInfo.IsDir() {
			// Path exists but is not a directory
			result.Status = "planned"
		} else {
			// Directory exists, check permissions
			if owner, hasOwner := desired["owner"].(string); hasOwner && runtime.GOOS != "windows" {
				currentOwner, err := p.getOwner(fileInfo)
				if err != nil {
					return nil, err
				}

				if currentOwner != owner {
					result.Status = "planned"
				}
			}

			if group, hasGroup := desired["group"].(string); hasGroup && runtime.GOOS != "windows" {
				currentGroup, err := p.getGroup(fileInfo)
				if err != nil {
					return nil, err
				}

				if currentGroup != group {
					result.Status = "planned"
				}
			}

			if mode, hasMode := desired["mode"].(string); hasMode && runtime.GOOS != "windows" {
				desiredMode, _ := strconv.ParseInt(mode, 8, 32)
				currentMode := fileInfo.Mode().Perm()

				if os.FileMode(desiredMode) != currentMode {
					result.Status = "planned"
				}
			}
		}

	case "present":
		content, hasContent := desired["content"].(string)
		source, hasSource := desired["source"].(string)

		if !exists {
			// File doesn't exist, needs to be created
			result.Status = "planned"
		} else if fileInfo.IsDir() {
			// Path exists but is a directory, not a file
			result.Status = "planned"
		} else if hasContent {
			// File exists, check if content matches
			currentContent, err := ioutil.ReadFile(path)
			if err != nil {
				return nil, err
			}

			if string(currentContent) != content {
				result.Status = "planned"
			}
		} else if hasSource {
			// File exists, check if content matches source
			currentMD5, err := p.calculateMD5(path)
			if err != nil {
				return nil, err
			}

			sourceMD5, err := p.calculateMD5(source)
			if err != nil {
				return nil, err
			}

			if currentMD5 != sourceMD5 {
				result.Status = "planned"
			}
		}

		// Check permissions for file
		if exists && !fileInfo.IsDir() && runtime.GOOS != "windows" {
			if owner, hasOwner := desired["owner"].(string); hasOwner {
				currentOwner, err := p.getOwner(fileInfo)
				if err != nil {
					return nil, err
				}

				if currentOwner != owner {
					result.Status = "planned"
				}
			}

			if group, hasGroup := desired["group"].(string); hasGroup {
				currentGroup, err := p.getGroup(fileInfo)
				if err != nil {
					return nil, err
				}

				if currentGroup != group {
					result.Status = "planned"
				}
			}

			if mode, hasMode := desired["mode"].(string); hasMode {
				desiredMode, _ := strconv.ParseInt(mode, 8, 32)
				currentMode := fileInfo.Mode().Perm()

				if os.FileMode(desiredMode) != currentMode {
					result.Status = "planned"
				}
			}
		}
	}

	return result, nil
}

// Apply creates, updates, or deletes a file
func (p *FileProvider) Apply(ctx context.Context, state *ResourceState) (*ResourceState, error) {
	path := state.Attributes["path"].(string)

	// Get desired state or default to "present"
	desiredState := "present"
	if state, ok := state.Attributes["state"].(string); ok {
		desiredState = state
	}

	result := &ResourceState{
		Type:       state.Type,
		Name:       state.Name,
		Attributes: state.Attributes,
		Status:     "unchanged",
	}

	// Check current state
	exists, fileInfo, err := p.fileExists(path)
	if err != nil {
		return nil, err
	}

	switch desiredState {
	case "absent":
		if exists {
			// Remove the file or directory
			if err := os.RemoveAll(path); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "deleted"
		}

	case "directory":
		if !exists {
			// Create the directory
			if err := os.MkdirAll(path, 0755); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "created"
		} else if !fileInfo.IsDir() {
			// Path exists but is not a directory, remove it and create directory
			if err := os.RemoveAll(path); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}

			if err := os.MkdirAll(path, 0755); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			result.Status = "updated"
		}

		// Set permissions for directory
		if runtime.GOOS != "windows" {
			if err := p.setPermissions(path, state.Attributes); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
		}

	case "present":
		content, hasContent := state.Attributes["content"].(string)
		source, hasSource := state.Attributes["source"].(string)

		// Determine if file needs to be created or updated
		needsUpdate := false

		if !exists {
			needsUpdate = true
		} else if fileInfo.IsDir() {
			// Path exists but is a directory, remove it
			if err := os.RemoveAll(path); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
			needsUpdate = true
		} else if hasContent {
			// Check if content matches
			currentContent, err := ioutil.ReadFile(path)
			if err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}

			if string(currentContent) != content {
				needsUpdate = true
			}
		} else if hasSource {
			// Check if content matches source
			currentMD5, err := p.calculateMD5(path)
			if err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}

			sourceMD5, err := p.calculateMD5(source)
			if err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}

			if currentMD5 != sourceMD5 {
				needsUpdate = true
			}
		}

		// Create or update file
		if needsUpdate {
			// Ensure parent directory exists
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}

			if hasContent {
				// Write content to file
				if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
					result.Status = "failed"
					result.Error = err
					return result, err
				}
			} else if hasSource {
				// Copy from source file
				sourceData, err := ioutil.ReadFile(source)
				if err != nil {
					result.Status = "failed"
					result.Error = err
					return result, err
				}

				if err := ioutil.WriteFile(path, sourceData, 0644); err != nil {
					result.Status = "failed"
					result.Error = err
					return result, err
				}
			}

			if exists {
				result.Status = "updated"
			} else {
				result.Status = "created"
			}
		}

		// Set permissions for file
		if runtime.GOOS != "windows" {
			if err := p.setPermissions(path, state.Attributes); err != nil {
				result.Status = "failed"
				result.Error = err
				return result, err
			}
		}
	}

	return result, nil
}

// getOwner gets the owner of a file
func (p *FileProvider) getOwner(fileInfo os.FileInfo) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("owner not supported on Windows")
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("failed to get file stats")
	}

	u, err := user.LookupId(fmt.Sprintf("%d", stat.Uid))
	if err != nil {
		return "", err
	}

	return u.Username, nil
}

// getGroup gets the group of a file
func (p *FileProvider) getGroup(fileInfo os.FileInfo) (string, error) {
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("group not supported on Windows")
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("failed to get file stats")
	}

	g, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid))
	if err != nil {
		return "", err
	}

	return g.Name, nil
}

// setPermissions sets the owner, group, and mode of a file or directory
func (p *FileProvider) setPermissions(path string, attributes map[string]interface{}) error {
	if runtime.GOOS == "windows" {
		return nil // Not supported on Windows
	}

	// Set owner and group
	if owner, hasOwner := attributes["owner"].(string); hasOwner {
		if group, hasGroup := attributes["group"].(string); hasGroup {
			// Get UID for owner
			u, err := user.Lookup(owner)
			if err != nil {
				return fmt.Errorf("failed to lookup owner '%s': %v", owner, err)
			}
			uid, _ := strconv.Atoi(u.Uid)

			// Get GID for group
			g, err := user.LookupGroup(group)
			if err != nil {
				return fmt.Errorf("failed to lookup group '%s': %v", group, err)
			}
			gid, _ := strconv.Atoi(g.Gid)

			// Change owner and group
			if err := os.Chown(path, uid, gid); err != nil {
				return fmt.Errorf("failed to change ownership to %s:%s: %v", owner, group, err)
			}
		} else {
			// Get UID for owner
			u, err := user.Lookup(owner)
			if err != nil {
				return fmt.Errorf("failed to lookup owner '%s': %v", owner, err)
			}
			uid, _ := strconv.Atoi(u.Uid)

			// Change owner only
			fileInfo, err := os.Stat(path)
			if err != nil {
				return err
			}

			stat, ok := fileInfo.Sys().(*syscall.Stat_t)
			if !ok {
				return fmt.Errorf("failed to get file stats")
			}

			if err := os.Chown(path, uid, int(stat.Gid)); err != nil {
				return fmt.Errorf("failed to change owner to %s: %v", owner, err)
			}
		}
	} else if group, hasGroup := attributes["group"].(string); hasGroup {
		// Get GID for group
		g, err := user.LookupGroup(group)
		if err != nil {
			return fmt.Errorf("failed to lookup group '%s': %v", group, err)
		}
		gid, _ := strconv.Atoi(g.Gid)

		// Change group only
		fileInfo, err := os.Stat(path)
		if err != nil {
			return err
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("failed to get file stats")
		}

		if err := os.Chown(path, int(stat.Uid), gid); err != nil {
			return fmt.Errorf("failed to change group to %s: %v", group, err)
		}
	}

	// Set mode
	if mode, hasMode := attributes["mode"].(string); hasMode {
		modeVal, _ := strconv.ParseInt(mode, 8, 32)
		if err := os.Chmod(path, os.FileMode(modeVal)); err != nil {
			return fmt.Errorf("failed to change mode to %s: %v", mode, err)
		}
	}

	return nil
}
