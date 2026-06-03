package lore

import "io/fs"

// Parse reads all files in the project and produces a World. It's the batch
// entry point used by the CLI; the LSP uses Index for incremental updates.
func Parse(project *Project) (*World, error) {
	files := make([]*FileParse, 0, len(project.FilePaths))
	for _, rel := range project.FilePaths {
		data, err := fs.ReadFile(project.FS, rel)
		if err != nil {
			continue
		}
		files = append(files, ParseFile(rel, string(data)))
	}
	world := Merge(files)
	defs := EffectiveRelationDefs(project.Config)
	world.Vocab = NewRelationVocab(defs, EffectivePlurals(project.Config))
	world.RelationIssues = ValidateRelations(defs)
	world.FileIssues = append(world.FileIssues, world.EdgeRemovalIssues(world.Vocab)...)
	return world, nil
}
