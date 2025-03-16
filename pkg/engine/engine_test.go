package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/dangerclosesec/zero/pkg/providers"
)

// MockProvider is a simple mock implementation of ResourceProvider
type MockProvider struct {
	ValidateFunc func(ctx context.Context, attributes map[string]interface{}) error
	PlanFunc     func(ctx context.Context, current, desired map[string]interface{}) (*providers.ResourceState, error)
	ApplyFunc    func(ctx context.Context, state *providers.ResourceState) (*providers.ResourceState, error)
}

func (m *MockProvider) Validate(ctx context.Context, attributes map[string]interface{}) error {
	if m.ValidateFunc != nil {
		return m.ValidateFunc(ctx, attributes)
	}
	return nil
}

func (m *MockProvider) Plan(ctx context.Context, current, desired map[string]interface{}) (*providers.ResourceState, error) {
	if m.PlanFunc != nil {
		return m.PlanFunc(ctx, current, desired)
	}
	return &providers.ResourceState{Status: "planned"}, nil
}

func (m *MockProvider) Apply(ctx context.Context, state *providers.ResourceState) (*providers.ResourceState, error) {
	if m.ApplyFunc != nil {
		return m.ApplyFunc(ctx, state)
	}
	return state, nil
}

func setupTestRegistry() *providers.ProviderRegistry {
	registry := providers.NewProviderRegistry()
	registry.Register("file", &MockProvider{})
	registry.Register("service", &MockProvider{})
	return registry
}

func TestNewEngine(t *testing.T) {
	registry := setupTestRegistry()
	engine := NewEngine(registry)

	if engine == nil {
		t.Fatal("Expected NewEngine to return a non-nil engine")
	}

	if engine.registry != registry {
		t.Error("Expected engine registry to match the provided registry")
	}

	if engine.platform == nil {
		t.Error("Expected engine platform to be initialized")
	}
}

func TestEngine_buildDependencyGraph(t *testing.T) {
	registry := setupTestRegistry()
	engine := NewEngine(registry)

	// Define some test resources with dependencies
	resources := []Resource{
		{
			Type: "file",
			Name: "file1",
			Attributes: map[string]interface{}{
				"path": "/path/to/file1",
			},
		},
		{
			Type: "file",
			Name: "file2",
			Attributes: map[string]interface{}{
				"path": "/path/to/file2",
			},
			DependsOn: []string{"file.file1"},
		},
		{
			Type: "service",
			Name: "service1",
			Attributes: map[string]interface{}{
				"name": "test-service",
			},
			DependsOn: []string{"file.file2"},
		},
	}

	// Build the graph
	graph, err := engine.buildDependencyGraph(resources)
	if err != nil {
		t.Fatalf("buildDependencyGraph returned error: %v", err)
	}

	// Check that all resources are in the graph
	if len(graph) != 3 {
		t.Errorf("Expected 3 nodes in the graph, got %d", len(graph))
	}

	// Check dependencies
	file1Node := graph["file.file1"]
	file2Node := graph["file.file2"]
	serviceNode := graph["service.service1"]

	if file1Node == nil || file2Node == nil || serviceNode == nil {
		t.Fatal("Expected all nodes to be in the graph")
	}

	// Check file1 has no dependencies
	if len(file1Node.DependsOn) != 0 {
		t.Errorf("Expected file1 to have no dependencies, got %d", len(file1Node.DependsOn))
	}

	// Check file1 has dependents
	if len(file1Node.DependedOnBy) != 1 {
		t.Errorf("Expected file1 to have 1 dependent, got %d", len(file1Node.DependedOnBy))
	}

	// Check file2 depends on file1
	if len(file2Node.DependsOn) != 1 {
		t.Errorf("Expected file2 to depend on 1 resource, got %d", len(file2Node.DependsOn))
	}

	// Check service depends on file2
	if len(serviceNode.DependsOn) != 1 {
		t.Errorf("Expected service to depend on 1 resource, got %d", len(serviceNode.DependsOn))
	}
}

func TestEngine_buildDependencyGraph_Error(t *testing.T) {
	registry := setupTestRegistry()
	engine := NewEngine(registry)

	// Define resources with a non-existent dependency
	resources := []Resource{
		{
			Type: "file",
			Name: "file1",
			Attributes: map[string]interface{}{
				"path": "/path/to/file1",
			},
			DependsOn: []string{"nonexistent.resource"},
		},
	}

	// Build the graph
	_, err := engine.buildDependencyGraph(resources)
	if err == nil {
		t.Error("Expected buildDependencyGraph to return an error for non-existent dependency")
	}
}

func TestEngine_topoSort(t *testing.T) {
	registry := setupTestRegistry()
	engine := NewEngine(registry)

	// Define some test resources with dependencies
	resources := []Resource{
		{
			Type: "file",
			Name: "file1",
			Attributes: map[string]interface{}{
				"path": "/path/to/file1",
			},
		},
		{
			Type: "file",
			Name: "file2",
			Attributes: map[string]interface{}{
				"path": "/path/to/file2",
			},
			DependsOn: []string{"file.file1"},
		},
		{
			Type: "service",
			Name: "service1",
			Attributes: map[string]interface{}{
				"name": "test-service",
			},
			DependsOn: []string{"file.file2"},
		},
	}

	// Build the graph
	graph, err := engine.buildDependencyGraph(resources)
	if err != nil {
		t.Fatalf("buildDependencyGraph returned error: %v", err)
	}

	// Sort the graph
	sorted, err := engine.topoSort(graph)
	if err != nil {
		t.Fatalf("topoSort returned error: %v", err)
	}

	// Check that all resources are in the sorted list
	if len(sorted) != 3 {
		t.Errorf("Expected 3 nodes in the sorted list, got %d", len(sorted))
	}

	// Check order (dependencies should come last after the reversal in the topoSort function)
	// The engine's topoSort reverses the list so that dependencies come first when applied
	if sorted[2].Resource.Name != "file1" {
		t.Errorf("Expected file1 to be last (after reversal), got %s", sorted[2].Resource.Name)
	}

	if sorted[1].Resource.Name != "file2" {
		t.Errorf("Expected file2 to be in the middle (after reversal), got %s", sorted[1].Resource.Name)
	}

	if sorted[0].Resource.Name != "service1" {
		t.Errorf("Expected service1 to be first (after reversal), got %s", sorted[0].Resource.Name)
	}
}

func TestEngine_topoSort_CycleDetection(t *testing.T) {
	registry := setupTestRegistry()
	engine := NewEngine(registry)

	// Create a graph with a cycle
	graphWithCycle := map[string]*ResourceNode{
		"file.file1": {
			Resource: Resource{
				Type: "file",
				Name: "file1",
				Attributes: map[string]interface{}{
					"path": "/path/to/file1",
				},
			},
		},
		"file.file2": {
			Resource: Resource{
				Type: "file",
				Name: "file2",
				Attributes: map[string]interface{}{
					"path": "/path/to/file2",
				},
			},
		},
	}

	// Add dependencies to create a cycle
	graphWithCycle["file.file1"].DependsOn = append(graphWithCycle["file.file1"].DependsOn, graphWithCycle["file.file2"])
	graphWithCycle["file.file2"].DependsOn = append(graphWithCycle["file.file2"].DependsOn, graphWithCycle["file.file1"])

	// Sort the graph with a cycle
	_, err := engine.topoSort(graphWithCycle)
	if err == nil {
		t.Error("Expected topoSort to detect a cycle and return an error")
	}
}

func TestEngine_validateResources(t *testing.T) {
	registry := providers.NewProviderRegistry()
	
	// Register a mock provider that validates specific attributes
	registry.Register("file", &MockProvider{
		ValidateFunc: func(ctx context.Context, attributes map[string]interface{}) error {
			if _, ok := attributes["path"]; !ok {
				return fmt.Errorf("validation error: file resource requires 'path' attribute")
			}
			return nil
		},
	})

	engine := NewEngine(registry)

	// Define valid resources
	validResources := []Resource{
		{
			Type: "file",
			Name: "file1",
			Attributes: map[string]interface{}{
				"path": "/path/to/file1",
			},
		},
	}

	// Build a graph with valid resources
	validGraph, _ := engine.buildDependencyGraph(validResources)

	// Validate resources
	err := engine.validateResources(context.Background(), validGraph)
	if err != nil {
		t.Errorf("validateResources returned error for valid resources: %v", err)
	}

	// Define invalid resources
	invalidResources := []Resource{
		{
			Type: "file",
			Name: "file2",
			Attributes: map[string]interface{}{
				// Missing required 'path' attribute
			},
		},
	}

	// Build a graph with invalid resources
	invalidGraph, _ := engine.buildDependencyGraph(invalidResources)

	// Validate resources
	err = engine.validateResources(context.Background(), invalidGraph)
	if err == nil {
		t.Error("Expected validateResources to return an error for invalid resources")
	}
}

func TestEngine_isPlatformSupported(t *testing.T) {
	registry := setupTestRegistry()
	engine := NewEngine(registry)

	// Resource with no platform condition
	resourceNoCondition := Resource{
		Type: "file",
		Name: "file1",
		Attributes: map[string]interface{}{
			"path": "/path/to/file1",
		},
	}
	
	// This should be supported everywhere
	if !engine.isPlatformSupported(resourceNoCondition) {
		t.Error("Expected resource with no platform condition to be supported")
	}

	// Resource with platform condition
	resourceWithCondition := Resource{
		Type: "file",
		Name: "file2",
		Attributes: map[string]interface{}{
			"path": "/path/to/file2",
		},
		Conditions: map[string][]string{
			"platform": {"linux", "darwin", "windows"},
		},
	}

	// This should be supported
	if !engine.isPlatformSupported(resourceWithCondition) {
		t.Error("Expected resource with matching platform condition to be supported")
	}

	// Resource with non-matching platform condition
	resourceNonMatching := Resource{
		Type: "file",
		Name: "file3",
		Attributes: map[string]interface{}{
			"path": "/path/to/file3",
		},
		Conditions: map[string][]string{
			"platform": {"invalid-platform"},
		},
	}

	// This should not be supported
	if engine.isPlatformSupported(resourceNonMatching) {
		t.Error("Expected resource with non-matching platform condition to not be supported")
	}
}

func TestEngine_Plan(t *testing.T) {
	registry := providers.NewProviderRegistry()
	
	// Register a mock provider
	registry.Register("file", &MockProvider{
		ValidateFunc: func(ctx context.Context, attributes map[string]interface{}) error {
			return nil
		},
		PlanFunc: func(ctx context.Context, current, desired map[string]interface{}) (*providers.ResourceState, error) {
			return &providers.ResourceState{
				Type:       "file",
				Name:       desired["path"].(string),
				Attributes: desired,
				Status:     "planned",
			}, nil
		},
	})

	engine := NewEngine(registry)

	// Define resources
	resources := []Resource{
		{
			Type: "file",
			Name: "file1",
			Attributes: map[string]interface{}{
				"path": "/path/to/file1",
			},
		},
	}

	// Execute Plan
	plan, err := engine.Plan(context.Background(), resources)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	// Check that the plan contains our resource
	if len(plan) != 1 {
		t.Errorf("Expected 1 resource in the plan, got %d", len(plan))
	}

	action, ok := plan["file.file1"]
	if !ok {
		t.Fatal("Expected plan to contain file.file1")
	}

	if action.Action != "create" {
		t.Errorf("Expected action to be 'create', got %s", action.Action)
	}
}

func TestEngine_Apply(t *testing.T) {
	registry := providers.NewProviderRegistry()
	
	// Register a mock provider
	registry.Register("file", &MockProvider{
		ValidateFunc: func(ctx context.Context, attributes map[string]interface{}) error {
			return nil
		},
		PlanFunc: func(ctx context.Context, current, desired map[string]interface{}) (*providers.ResourceState, error) {
			return &providers.ResourceState{
				Type:       "file",
				Name:       desired["path"].(string),
				Attributes: desired,
				Status:     "planned",
			}, nil
		},
		ApplyFunc: func(ctx context.Context, state *providers.ResourceState) (*providers.ResourceState, error) {
			return &providers.ResourceState{
				Type:       state.Type,
				Name:       state.Name,
				Attributes: state.Attributes,
				Status:     "created",
			}, nil
		},
	})

	engine := NewEngine(registry)

	// Define resources
	resources := []Resource{
		{
			Type: "file",
			Name: "file1",
			Attributes: map[string]interface{}{
				"path": "/path/to/file1",
			},
		},
	}

	// Execute Apply
	results, err := engine.Apply(context.Background(), resources)
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	// Check that the results contain our resource
	if len(results) != 1 {
		t.Errorf("Expected 1 resource in the results, got %d", len(results))
	}

	state, ok := results["file.file1"]
	if !ok {
		t.Fatal("Expected results to contain file.file1")
	}

	if state.Status != "created" {
		t.Errorf("Expected status to be 'created', got %s", state.Status)
	}
}