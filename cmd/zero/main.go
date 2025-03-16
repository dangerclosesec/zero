package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dangerclosesec/zero/pkg/engine"
	"github.com/dangerclosesec/zero/pkg/parser"
	"github.com/dangerclosesec/zero/pkg/providers"
)

func main() {
	// Define command line flags
	applyCmd := flag.Bool("apply", false, "Apply the configuration")
	planCmd := flag.Bool("plan", false, "Show what would be changed")
	configFile := flag.String("config", "", "Path to the configuration file")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	if *configFile == "" {
		fmt.Println("Error: No configuration file specified")
		flag.Usage()
		os.Exit(1)
	}

	// Initialize logger
	if *verbose {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	} else {
		log.SetFlags(0)
	}

	// Get absolute path of config file for includes
	absConfigPath, err := filepath.Abs(*configFile)
	if err != nil {
		log.Fatalf("Error resolving config path: %v", err)
	}
	configDir := filepath.Dir(absConfigPath)

	// Process includes and variables
	includeHandler := parser.NewIncludeHandler(configDir)
	resources, err := includeHandler.ProcessIncludes(absConfigPath)
	if err != nil {
		log.Fatalf("Error processing configuration: %v", err)
	}

	// Process templates
	processedResources, err := includeHandler.ProcessTemplates(resources)
	if err != nil {
		log.Fatalf("Error processing templates: %v", err)
	}

	// Convert parser.Resource to engine.Resource
	engineResources := make([]engine.Resource, len(processedResources))
	for i, r := range processedResources {
		engineResources[i] = engine.Resource{
			Type:       r.Type,
			Name:       r.Name,
			Attributes: r.Attributes,
			DependsOn:  r.DependsOn,
			Conditions: r.Conditions,
		}
	}

	// Create provider registry
	registry := providers.NewProviderRegistry()

	// Register providers
	registry.Register("file", providers.NewFileProvider())
	registry.Register("package", providers.NewPackageProvider())
	registry.Register("service", providers.NewServiceProvider())
	registry.Register("windows_feature", providers.NewWindowsFeatureProvider())

	// Create engine
	e := engine.NewEngine(registry)

	// Create context
	ctx := context.Background()

	if *planCmd {
		// Plan mode - show what changes would be made
		fmt.Println("Planning configuration changes...")
		startTime := time.Now()

		plan, err := e.Plan(ctx, engineResources)
		if err != nil {
			log.Fatalf("Error planning configuration: %v", err)
		}

		// Print plan
		fmt.Println("\nPlan:")
		fmt.Println(strings.Repeat("-", 60))

		add := 0
		change := 0
		destroy := 0

		for id, action := range plan {
			switch action.Action {
			case "create":
				fmt.Printf("+ create: %s\n", id)
				if *verbose {
					fmt.Printf("    %s\n", action.Details)
				}
				add++
			case "update":
				fmt.Printf("~ update: %s\n", id)
				if *verbose {
					fmt.Printf("    %s\n", action.Details)
				}
				change++
			case "delete":
				fmt.Printf("- delete: %s\n", id)
				if *verbose {
					fmt.Printf("    %s\n", action.Details)
				}
				destroy++
			case "no-op":
				if *verbose {
					fmt.Printf("  no-op: %s\n", id)
				}
			}
		}

		fmt.Println(strings.Repeat("-", 60))
		duration := time.Since(startTime)
		fmt.Printf("Plan: %d to add, %d to change, %d to destroy (in %v)\n",
			add, change, destroy, duration)

	} else if *applyCmd {
		// Apply mode
		fmt.Println("Applying configuration...")
		startTime := time.Now()

		results, err := e.Apply(ctx, engineResources)
		if err != nil {
			log.Fatalf("Error applying configuration: %v", err)
		}

		// Print results
		fmt.Println("\nResults:")
		fmt.Println(strings.Repeat("-", 60))

		success := 0
		failed := 0
		skipped := 0

		for id, state := range results {
			switch state.Status {
			case "created", "updated":
				fmt.Printf("✓ %s: %s\n", id, state.Status)
				success++
			case "unchanged":
				if *verbose {
					fmt.Printf("- %s: %s\n", id, state.Status)
				}
				skipped++
			case "failed":
				fmt.Printf("✗ %s: %s (%v)\n", id, state.Status, state.Error)
				failed++
			}
		}

		fmt.Println(strings.Repeat("-", 60))
		duration := time.Since(startTime)
		fmt.Printf("Applied %d resources in %v\n", len(results), duration)
		fmt.Printf("Success: %d, Failed: %d, Skipped: %d\n", success, failed, skipped)

		if failed > 0 {
			os.Exit(1)
		}
	} else {
		fmt.Println("No action specified. Use --plan or --apply")
		flag.Usage()
		os.Exit(1)
	}
}
