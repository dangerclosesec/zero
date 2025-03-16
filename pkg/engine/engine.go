package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/dangerclosesec/zero/pkg/providers"
)

// ResourceNode represents a resource in the dependency graph
type ResourceNode struct {
	Resource      Resource
	DependsOn     []*ResourceNode
	DependedOnBy  []*ResourceNode
	State         *providers.ResourceState
	Visited       bool
	Applied       bool
	ExecutionTime time.Time
}

// Resource represents a parsed resource
type Resource struct {
	Type       string
	Name       string
	Attributes map[string]interface{}
	DependsOn  []string
	Conditions map[string][]string
}

// PlanAction represents a planned action for a resource
type PlanAction struct {
	Action  string // "create", "update", "delete", "no-op"
	Details string
}

// Engine is the core execution engine for configurations
type Engine struct {
	registry *providers.ProviderRegistry
	platform *providers.PlatformChecker
}

// NewEngine creates a new execution engine
func NewEngine(registry *providers.ProviderRegistry) *Engine {
	return &Engine{
		registry: registry,
		platform: &providers.PlatformChecker{},
	}
}

// Plan generates a plan of changes without applying them
func (e *Engine) Plan(ctx context.Context, resources []Resource) (map[string]PlanAction, error) {
	// Build dependency graph
	graph, err := e.buildDependencyGraph(resources)
	if err != nil {
		return nil, err
	}

	// Validate all resources
	if err := e.validateResources(ctx, graph); err != nil {
		return nil, err
	}

	// Sort resources by dependency order
	orderedNodes, err := e.topoSort(graph)
	if err != nil {
		return nil, err
	}

	// Plan changes for each resource
	results := make(map[string]PlanAction)
	for _, node := range orderedNodes {
		// Skip resources that don't apply to this platform
		if !e.isPlatformSupported(node.Resource) {
			continue
		}

		resourceID := fmt.Sprintf("%s.%s", node.Resource.Type, node.Resource.Name)

		// Get the provider for this resource type
		provider, err := e.registry.Get(node.Resource.Type)
		if err != nil {
			results[resourceID] = PlanAction{
				Action:  "error",
				Details: fmt.Sprintf("Error getting provider: %v", err),
			}
			continue
		}

		// Plan the resource
		current := make(map[string]interface{}) // In a real system, this would be loaded from state
		planned, err := provider.Plan(ctx, current, node.Resource.Attributes)
		if err != nil {
			results[resourceID] = PlanAction{
				Action:  "error",
				Details: fmt.Sprintf("Error planning: %v", err),
			}
			continue
		}

		// Determine the action based on the status
		action := "no-op"
		details := "No changes required"

		switch planned.Status {
		case "planned":
			// Check if this is a new resource or an update
			if _, exists := current["path"]; exists {
				action = "update"
				details = "Resource will be updated"
			} else {
				action = "create"
				details = "Resource will be created"
			}
		case "unchanged":
			action = "no-op"
			details = "Resource already in desired state"
		}

		results[resourceID] = PlanAction{
			Action:  action,
			Details: details,
		}
	}

	return results, nil
}

// Apply applies the given resources
func (e *Engine) Apply(ctx context.Context, resources []Resource) (map[string]*providers.ResourceState, error) {
	// Build dependency graph
	graph, err := e.buildDependencyGraph(resources)
	if err != nil {
		return nil, err
	}

	// Validate all resources
	if err := e.validateResources(ctx, graph); err != nil {
		return nil, err
	}

	// Sort resources by dependency order
	orderedNodes, err := e.topoSort(graph)
	if err != nil {
		return nil, err
	}

	// Apply resources in order
	results := make(map[string]*providers.ResourceState)
	for _, node := range orderedNodes {
		// Skip resources that don't apply to this platform
		if !e.isPlatformSupported(node.Resource) {
			fmt.Printf("Skipping resource %s.%s (platform not supported)\n",
				node.Resource.Type, node.Resource.Name)
			continue
		}

		resourceID := fmt.Sprintf("%s.%s", node.Resource.Type, node.Resource.Name)

		// Get the provider for this resource type
		provider, err := e.registry.Get(node.Resource.Type)
		if err != nil {
			fmt.Printf("Error getting provider for %s: %v\n", resourceID, err)
			results[resourceID] = &providers.ResourceState{
				Type:   node.Resource.Type,
				Name:   node.Resource.Name,
				Status: "failed",
				Error:  err,
			}
			continue
		}

		// Plan the resource
		current := make(map[string]interface{}) // In a real system, this would be loaded from state
		planned, err := provider.Plan(ctx, current, node.Resource.Attributes)
		if err != nil {
			fmt.Printf("Error planning %s: %v\n", resourceID, err)
			results[resourceID] = &providers.ResourceState{
				Type:   node.Resource.Type,
				Name:   node.Resource.Name,
				Status: "failed",
				Error:  err,
			}
			continue
		}

		// Apply the resource
		fmt.Printf("Applying %s\n", resourceID)
		state, err := provider.Apply(ctx, planned)
		if err != nil {
			fmt.Printf("Error applying %s: %v\n", resourceID, err)
			state = &providers.ResourceState{
				Type:       node.Resource.Type,
				Name:       node.Resource.Name,
				Attributes: node.Resource.Attributes,
				Status:     "failed",
				Error:      err,
			}
		}

		results[resourceID] = state
		node.State = state
		node.Applied = true
	}

	return results, nil
}

// buildDependencyGraph builds a dependency graph from resources
func (e *Engine) buildDependencyGraph(resources []Resource) (map[string]*ResourceNode, error) {
	graph := make(map[string]*ResourceNode)

	// First pass: create nodes
	for _, resource := range resources {
		id := fmt.Sprintf("%s.%s", resource.Type, resource.Name)
		graph[id] = &ResourceNode{
			Resource:     resource,
			DependsOn:    []*ResourceNode{},
			DependedOnBy: []*ResourceNode{},
		}
	}

	// Second pass: link dependencies
	for _, resource := range resources {
		id := fmt.Sprintf("%s.%s", resource.Type, resource.Name)
		node := graph[id]

		for _, depID := range resource.DependsOn {
			depNode, exists := graph[depID]
			if !exists {
				return nil, fmt.Errorf("resource %s depends on non-existent resource %s", id, depID)
			}

			node.DependsOn = append(node.DependsOn, depNode)
			depNode.DependedOnBy = append(depNode.DependedOnBy, node)
		}
	}

	return graph, nil
}

// validateResources validates all resources in the graph
func (e *Engine) validateResources(ctx context.Context, graph map[string]*ResourceNode) error {
	for id, node := range graph {
		// Skip resources that don't apply to this platform
		if !e.isPlatformSupported(node.Resource) {
			continue
		}

		provider, err := e.registry.Get(node.Resource.Type)
		if err != nil {
			return fmt.Errorf("no provider for resource %s: %v", id, err)
		}

		if _, ok := node.Resource.Attributes["name"]; !ok {
			node.Resource.Attributes["name"] = node.Resource.Name
		}

		if err := provider.Validate(ctx, node.Resource.Attributes); err != nil {
			return fmt.Errorf("validation failed for resource %s: %v", id, err)
		}
	}

	return nil
}

// topoSort performs a topological sort of the dependency graph
func (e *Engine) topoSort(graph map[string]*ResourceNode) ([]*ResourceNode, error) {
	result := []*ResourceNode{}
	visited := make(map[string]bool)

	var visit func(node *ResourceNode) error
	visit = func(node *ResourceNode) error {
		id := fmt.Sprintf("%s.%s", node.Resource.Type, node.Resource.Name)

		// Check for cycles
		if node.Visited {
			// We're in a cycle
			return fmt.Errorf("dependency cycle detected involving resource %s", id)
		}

		// Skip if already processed
		if visited[id] {
			return nil
		}

		node.Visited = true

		// Visit dependencies first
		for _, dep := range node.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}

		node.Visited = false
		visited[id] = true
		result = append(result, node)
		return nil
	}

	// Visit all nodes
	for _, node := range graph {
		if !visited[fmt.Sprintf("%s.%s", node.Resource.Type, node.Resource.Name)] {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}

	// Reverse the result since we want dependencies first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}

// isPlatformSupported checks if the resource is supported on the current platform
func (e *Engine) isPlatformSupported(resource Resource) bool {
	platforms, exists := resource.Conditions["platform"]
	if !exists {
		// No platform condition, so it's supported everywhere
		return true
	}

	return e.platform.IsSupported(platforms)
}
