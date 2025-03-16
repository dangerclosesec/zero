package providers

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileProvider_Validate(t *testing.T) {
	provider := NewFileProvider()
	ctx := context.Background()

	// Test valid minimal attributes
	validAttrs := map[string]interface{}{
		"path": "/path/to/file",
	}
	if err := provider.Validate(ctx, validAttrs); err != nil {
		t.Errorf("Expected no error for valid attributes, got: %v", err)
	}

	// Test missing path attribute
	invalidAttrs := map[string]interface{}{
		"content": "test content",
	}
	if err := provider.Validate(ctx, invalidAttrs); err == nil {
		t.Error("Expected error for missing path attribute, got nil")
	}

	// Test invalid path type
	invalidPathAttrs := map[string]interface{}{
		"path": 123, // should be string
	}
	if err := provider.Validate(ctx, invalidPathAttrs); err == nil {
		t.Error("Expected error for invalid path type, got nil")
	}

	// Test mutually exclusive attributes
	mutuallyExclusiveAttrs := map[string]interface{}{
		"path":    "/path/to/file",
		"content": "test content",
		"source":  "/path/to/source",
	}
	if err := provider.Validate(ctx, mutuallyExclusiveAttrs); err == nil {
		t.Error("Expected error for mutually exclusive content and source, got nil")
	}

	// Test invalid state
	invalidStateAttrs := map[string]interface{}{
		"path":  "/path/to/file",
		"state": "invalid-state",
	}
	if err := provider.Validate(ctx, invalidStateAttrs); err == nil {
		t.Error("Expected error for invalid state, got nil")
	}

	// Test invalid mode
	invalidModeAttrs := map[string]interface{}{
		"path": "/path/to/file",
		"mode": "not-octal",
	}
	if err := provider.Validate(ctx, invalidModeAttrs); err == nil {
		t.Error("Expected error for invalid mode, got nil")
	}

	// Test valid mode
	validModeAttrs := map[string]interface{}{
		"path": "/path/to/file",
		"mode": "0755",
	}
	if err := provider.Validate(ctx, validModeAttrs); err != nil {
		t.Errorf("Expected no error for valid mode, got: %v", err)
	}
}

func TestFileProvider_Plan_FilePresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to permission differences")
	}

	// Create a temporary directory and file for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test_file.txt")
	if err := ioutil.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider := NewFileProvider()
	ctx := context.Background()

	// Test planning with existing file - should be unchanged
	current := map[string]interface{}{"path": testFilePath}
	desired := map[string]interface{}{
		"path":    testFilePath,
		"content": "test content", // same as existing file
	}

	result, err := provider.Plan(ctx, current, desired)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "unchanged" {
		t.Errorf("Expected status 'unchanged', got '%s'", result.Status)
	}

	// Test planning with different content - should be planned
	desired["content"] = "new content"
	result, err = provider.Plan(ctx, current, desired)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "planned" {
		t.Errorf("Expected status 'planned', got '%s'", result.Status)
	}
}

func TestFileProvider_Plan_FileAbsent(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Path to a file that does not exist
	nonExistentFilePath := filepath.Join(tempDir, "nonexistent_file.txt")

	provider := NewFileProvider()
	ctx := context.Background()

	// Test planning with non-existent file - should be planned
	current := map[string]interface{}{}
	desired := map[string]interface{}{
		"path":    nonExistentFilePath,
		"content": "new content",
	}

	result, err := provider.Plan(ctx, current, desired)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "planned" {
		t.Errorf("Expected status 'planned', got '%s'", result.Status)
	}

	// Test planning with state=absent for non-existent file - should be unchanged
	desired["state"] = "absent"
	result, err = provider.Plan(ctx, current, desired)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "unchanged" {
		t.Errorf("Expected status 'unchanged', got '%s'", result.Status)
	}
}

func TestFileProvider_Plan_Directory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a subdirectory
	testDirPath := filepath.Join(tempDir, "test_dir")
	if err := os.Mkdir(testDirPath, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	provider := NewFileProvider()
	ctx := context.Background()

	// Test planning with existing directory - should be unchanged
	current := map[string]interface{}{"path": testDirPath}
	desired := map[string]interface{}{
		"path":  testDirPath,
		"state": "directory",
	}

	result, err := provider.Plan(ctx, current, desired)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "unchanged" {
		t.Errorf("Expected status 'unchanged', got '%s'", result.Status)
	}

	// Test planning with file where we want directory - should be planned
	testFilePath := filepath.Join(tempDir, "test_file.txt")
	if err := ioutil.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	desired["path"] = testFilePath
	result, err = provider.Plan(ctx, map[string]interface{}{"path": testFilePath}, desired)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	if result.Status != "planned" {
		t.Errorf("Expected status 'planned', got '%s'", result.Status)
	}
}

func TestFileProvider_Apply_CreateFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Path to a file that does not exist
	testFilePath := filepath.Join(tempDir, "test_file.txt")

	provider := NewFileProvider()
	ctx := context.Background()

	// Create a test state for a file that should be created
	state := &ResourceState{
		Type: "file",
		Name: "test_file",
		Attributes: map[string]interface{}{
			"path":    testFilePath,
			"content": "test content",
		},
		Status: "planned",
	}

	// Apply the state to create the file
	result, err := provider.Apply(ctx, state)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "created" {
		t.Errorf("Expected status 'created', got '%s'", result.Status)
	}

	// Verify the file was created
	content, err := ioutil.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("Expected file content 'test content', got '%s'", string(content))
	}
}

func TestFileProvider_Apply_CreateDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Path to a directory that does not exist
	testDirPath := filepath.Join(tempDir, "test_dir")

	provider := NewFileProvider()
	ctx := context.Background()

	// Create a test state for a directory that should be created
	state := &ResourceState{
		Type: "file",
		Name: "test_dir",
		Attributes: map[string]interface{}{
			"path":  testDirPath,
			"state": "directory",
		},
		Status: "planned",
	}

	// Apply the state to create the directory
	result, err := provider.Apply(ctx, state)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "created" {
		t.Errorf("Expected status 'created', got '%s'", result.Status)
	}

	// Verify the directory was created
	fileInfo, err := os.Stat(testDirPath)
	if err != nil {
		t.Fatalf("Failed to stat created directory: %v", err)
	}

	if !fileInfo.IsDir() {
		t.Error("Expected created path to be a directory")
	}
}

func TestFileProvider_Apply_RemoveFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test_file.txt")
	if err := ioutil.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider := NewFileProvider()
	ctx := context.Background()

	// Create a test state for a file that should be removed
	state := &ResourceState{
		Type: "file",
		Name: "test_file",
		Attributes: map[string]interface{}{
			"path":  testFilePath,
			"state": "absent",
		},
		Status: "planned",
	}

	// Apply the state to remove the file
	result, err := provider.Apply(ctx, state)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "deleted" {
		t.Errorf("Expected status 'deleted', got '%s'", result.Status)
	}

	// Verify the file was removed
	_, err = os.Stat(testFilePath)
	if !os.IsNotExist(err) {
		t.Error("Expected file to be removed, but it still exists")
	}
}

func TestFileProvider_Apply_UpdateFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test_file.txt")
	if err := ioutil.WriteFile(testFilePath, []byte("original content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	provider := NewFileProvider()
	ctx := context.Background()

	// Create a test state for a file that should be updated
	state := &ResourceState{
		Type: "file",
		Name: "test_file",
		Attributes: map[string]interface{}{
			"path":    testFilePath,
			"content": "updated content",
		},
		Status: "planned",
	}

	// Apply the state to update the file
	result, err := provider.Apply(ctx, state)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if result.Status != "updated" {
		t.Errorf("Expected status 'updated', got '%s'", result.Status)
	}

	// Verify the file was updated
	content, err := ioutil.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	if string(content) != "updated content" {
		t.Errorf("Expected file content 'updated content', got '%s'", string(content))
	}
}

// Utility functions used by FileProvider
func TestFileProvider_UtilityFunctions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to permission differences")
	}
	
	provider := NewFileProvider()

	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "file_provider_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test fileExists with non-existent file
	exists, _, err := provider.fileExists(filepath.Join(tempDir, "nonexistent.txt"))
	if err != nil {
		t.Errorf("fileExists returned error for non-existent file: %v", err)
	}
	if exists {
		t.Error("Expected fileExists to return false for non-existent file")
	}

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test_file.txt")
	if err := ioutil.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test fileExists with existing file
	exists, fileInfo, err := provider.fileExists(testFilePath)
	if err != nil {
		t.Errorf("fileExists returned error for existing file: %v", err)
	}
	if !exists {
		t.Error("Expected fileExists to return true for existing file")
	}
	if fileInfo == nil {
		t.Error("Expected fileExists to return non-nil fileInfo for existing file")
	}

	// Test calculateMD5
	md5, err := provider.calculateMD5(testFilePath)
	if err != nil {
		t.Errorf("calculateMD5 returned error: %v", err)
	}
	if md5 == "" {
		t.Error("Expected calculateMD5 to return non-empty string")
	}
}