package main

import (
	"fmt"
	"os"
	"strings"

	"lore/internal/lore"
	"lore/internal/lsp"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Commands that run without a loaded project.
	if cmd == "config" {
		cmdConfig(args)
		return
	}
	if cmd == "lsp" {
		s := lsp.NewServer()
		if err := s.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "LSP error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	project, err := lore.FindAndLoad()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	world, err := lore.Parse(project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "list":
		cmdList(world)
	case "query":
		cmdQuery(world, args)
	case "refs":
		cmdRefs(world, args)
	case "search":
		cmdSearch(world, args)
	case "check":
		cmdCheck(world)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`Usage: lore <command> [args]

Commands:
  list              List all entities
  query <name>      Show entity description and references
  refs <name>       Show all references to an entity
  search <text>     Full-text search across all files
  check             Report undefined references
  config init       Create a lore.toml in the current directory
`)
}

func cmdConfig(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: lore config <subcommand>\n\nSubcommands:\n  init    Create a lore.toml in the current directory")
		os.Exit(1)
	}

	switch args[0] {
	case "init":
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := lore.InitConfig(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created lore.toml")
	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdList(world *lore.World) {
	for _, ent := range world.Entities {
		if ent.Type != "" {
			fmt.Printf("%s (%s)\n", ent.Name, ent.Type)
		} else {
			fmt.Println(ent.Name)
		}
	}
}

func cmdQuery(world *lore.World, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: lore query <name>")
		os.Exit(1)
	}

	name := args[0]
	ent, err := world.FindEntity(name)
	if err != nil {
		if ambig, ok := err.(*lore.AmbiguousError); ok {
			fmt.Fprintf(os.Stderr, "\"%s\" is ambiguous. Did you mean:\n", name)
			for _, m := range ambig.Matches {
				if m.Type != "" {
					fmt.Fprintf(os.Stderr, "  %s (%s)\n", m.Name, m.Type)
				} else {
					fmt.Fprintf(os.Stderr, "  %s\n", m.Name)
				}
			}
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Entity not found: %s\n", name)
		os.Exit(1)
	}

	if ent.Type != "" {
		fmt.Printf("# %s (%s)\n\n", ent.Name, ent.Type)
	} else {
		fmt.Printf("# %s\n\n", ent.Name)
	}

	if len(ent.Aliases) > 0 {
		fmt.Printf("Also known as: %s\n\n", strings.Join(ent.Aliases, ", "))
	}

	if state := lore.FormatStateBlock(ent.Tags, ent.Fields); state != "" {
		fmt.Println(state)
		fmt.Println()
	}

	for _, desc := range ent.Descriptions {
		fmt.Println(desc.Text)
		fmt.Printf("  — %s:%d\n\n", desc.File, desc.Line)
	}

	refs := world.GetReferences(ent.Name)
	if len(refs) > 0 {
		fmt.Println("Referenced by:")
		for _, ref := range refs {
			source := ref.SourceEntity
			if source == "" {
				source = "(free text)"
			}
			fmt.Printf("  %s — %s:%d\n", source, ref.File, ref.Line)
		}
	}
}

func cmdRefs(world *lore.World, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: lore refs <name>")
		os.Exit(1)
	}

	name := args[0]
	refs := world.GetReferences(name)
	if len(refs) == 0 {
		fmt.Printf("No references to \"%s\".\n", name)
		return
	}

	for _, ref := range refs {
		fmt.Printf("%s:%d — %s\n", ref.File, ref.Line, ref.Context)
	}
}

func cmdSearch(world *lore.World, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: lore search <text>")
		os.Exit(1)
	}

	query := args[0]
	results := world.Search(query)
	if len(results) == 0 {
		fmt.Printf("No results for \"%s\".\n", query)
		return
	}

	for _, r := range results {
		fmt.Printf("%s:%d: %s\n", r.File, r.Line, r.Context)
	}
}

func cmdCheck(world *lore.World) {
	issues := world.Check()
	if len(issues) == 0 {
		fmt.Println("No issues found.")
		return
	}

	for _, issue := range issues {
		fmt.Printf("%s:%d: %s\n", issue.File, issue.Line, issue.Message)
	}
}
