package parser

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"
)

// IncludeHandler manages file inclusions and platform-specific includes
type IncludeHandler struct {
	BasePath       string
	ProcessedFiles map[string]bool
	Variables      map[string]string
	Templates      map[string]string
}

// NewIncludeHandler creates a new include handler
func NewIncludeHandler(basePath string) *IncludeHandler {
	return &IncludeHandler{
		BasePath:       basePath,
		ProcessedFiles: make(map[string]bool),
		Variables:      make(map[string]string),
		Templates:      make(map[string]string),
	}
}

// SetVariable sets a variable value
func (h *IncludeHandler) SetVariable(name, value string) {
	h.Variables[name] = value
}

// GetVariable gets a variable value
func (h *IncludeHandler) GetVariable(name string) (string, bool) {
	value, exists := h.Variables[name]
	return value, exists
}

// SetTemplate sets a template value
func (h *IncludeHandler) SetTemplate(name, content string) {
	h.Templates[name] = content
}

// GetTemplate gets a template content
func (h *IncludeHandler) GetTemplate(name string) (string, bool) {
	content, exists := h.Templates[name]
	return content, exists
}

// ReplaceVariables replaces variables in a string with their values
func (h *IncludeHandler) ReplaceVariables(content string) string {
	// Replace all occurrences of $variable with the variable value
	for name, value := range h.Variables {
		content = strings.ReplaceAll(content, "$"+name, value)
	}
	return content
}

// ProcessIncludes processes include statements in a configuration file
func (h *IncludeHandler) ProcessIncludes(configFile string) ([]Resource, error) {
	allResources := []Resource{}

	// Check if we've already processed this file to avoid cycles
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		return nil, fmt.Errorf("error resolving absolute path for %s: %v", configFile, err)
	}

	if h.ProcessedFiles[absPath] {
		// Already processed, skip
		return allResources, nil
	}

	// Mark as processed
	h.ProcessedFiles[absPath] = true

	// Read the file
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s: %v", configFile, err)
	}

	// Parse the file
	parser := NewParser(strings.NewReader(string(data)))
	fileResources, err := parser.Parse()
	if err != nil {
		for _, parseErr := range parser.Errors() {
			fmt.Printf("Parse error in %s: %s\n", configFile, parseErr)
		}
		return nil, fmt.Errorf("error parsing config file %s: %v", configFile, err)
	}

	// Process resources from this file
	for _, resource := range fileResources {
		// Handle special resource types
		switch resource.Type {
		case "include":
			// Regular include
			if pattern, ok := resource.Attributes["path"].(string); ok {
				includePath := h.resolveIncludePath(configFile, pattern)
				matches, err := filepath.Glob(includePath)
				if err != nil {
					return nil, fmt.Errorf("error resolving include pattern %s: %v", pattern, err)
				}

				if len(matches) == 0 {
					fmt.Printf("Warning: no files matched include pattern %s\n", pattern)
				}

				for _, match := range matches {
					includeResources, err := h.ProcessIncludes(match)
					if err != nil {
						return nil, err
					}
					allResources = append(allResources, includeResources...)
				}
			}

		case "include_platform":
			// Platform-specific include
			platformPath := ""

			// Find the pattern for the current platform
			switch runtime.GOOS {
			case "linux":
				if pattern, ok := resource.Attributes["linux"].(string); ok {
					platformPath = pattern
				}
			case "darwin":
				if pattern, ok := resource.Attributes["darwin"].(string); ok {
					platformPath = pattern
				}
			case "windows":
				if pattern, ok := resource.Attributes["windows"].(string); ok {
					platformPath = pattern
				}
			}

			if platformPath != "" {
				includePath := h.resolveIncludePath(configFile, platformPath)
				matches, err := filepath.Glob(includePath)
				if err != nil {
					return nil, fmt.Errorf("error resolving platform include pattern %s: %v", platformPath, err)
				}

				if len(matches) == 0 {
					fmt.Printf("Warning: no files matched platform-specific include pattern %s\n", platformPath)
				}

				for _, match := range matches {
					includeResources, err := h.ProcessIncludes(match)
					if err != nil {
						return nil, err
					}
					allResources = append(allResources, includeResources...)
				}
			}

		case "variable":
			// Variable definition
			name := resource.Name
			if value, ok := resource.Attributes["value"].(string); ok {
				// Resolve any variables in the value itself
				resolvedValue := h.ReplaceVariables(value)
				h.SetVariable(name, resolvedValue)
			}

		case "template":
			// Template definition
			name := resource.Name
			if content, ok := resource.Attributes["content"].(string); ok {
				h.SetTemplate(name, content)
			}

		default:
			// Regular resource, process variable substitutions in string attributes
			processedResource := resource

			// Process all string attributes for variable substitution
			for key, value := range processedResource.Attributes {
				if strValue, ok := value.(string); ok {
					processedResource.Attributes[key] = h.ReplaceVariables(strValue)
				}
			}

			allResources = append(allResources, processedResource)
		}
	}

	return allResources, nil
}

// resolveIncludePath resolves an include path relative to the including file
func (h *IncludeHandler) resolveIncludePath(baseFile, includePath string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}

	baseDir := filepath.Dir(baseFile)
	return filepath.Join(baseDir, includePath)
}

// ProcessTemplates processes template functions in resources
func (h *IncludeHandler) ProcessTemplates(resources []Resource) ([]Resource, error) {
	result := make([]Resource, len(resources))
	copy(result, resources)

	// Process all string attributes for template functions
	for i, resource := range result {
		for key, value := range resource.Attributes {
			if strValue, ok := value.(string); ok {
				// Check for template function: template("name")
				if strings.HasPrefix(strValue, "template(") && strings.HasSuffix(strValue, ")") {
					templateName := strValue[9 : len(strValue)-1]
					if content, exists := h.GetTemplate(templateName); exists {
						// Replace variables in the template content
						processed := h.ReplaceVariables(content)
						result[i].Attributes[key] = processed
					}
				} else if strings.HasPrefix(strValue, "file(") && strings.HasSuffix(strValue, ")") {
					// Check for file function: file("path/to/file")
					filePath := strValue[5 : len(strValue)-1]
					resolved := h.resolveIncludePath(h.BasePath, filePath)
					data, err := ioutil.ReadFile(resolved)
					if err != nil {
						return nil, fmt.Errorf("error reading file %s: %v", filePath, err)
					}
					// Replace variables in the file content
					content := string(data)
					processed := h.ReplaceVariables(content)
					result[i].Attributes[key] = processed
				}
			}
		}
	}

	return result, nil
}
