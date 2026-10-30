package main

import (
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree/v2"
	"github.com/spf13/cobra"
)

const skillCommandOutput = "Install the tag-track skill using the Skills CLI:\n\n  npx skills add alexgorbatchev/tag-track-cli --skill tag-track\n\nSkill:\n\n  https://github.com/alexgorbatchev/tag-track-cli/tree/main/skills/tag-track\n"

var techCatalog = cobrahelptree.TechCatalog{
	"tag-track": {Args: []cobrahelptree.ArgSpec{{Name: "[file...]", Description: "Audio paths, or - to read newline-delimited paths from stdin"}}, Env: []cobrahelptree.EnvSpec{
		{Name: "AGENT", Description: "1, true, yes select compact output"},
		{Name: "TAG_TRACK_PROGRESS_TARGET", Description: "Progress destination when flags are omitted"},
		{Name: "XDG_CACHE_HOME", Description: "Cache root directory"},
	}},
	"tag-track track update":         {Args: []cobrahelptree.ArgSpec{{Name: "[file...]", Description: "Audio paths, or - to read paths from stdin"}}},
	"tag-track track inspect":        {Args: []cobrahelptree.ArgSpec{{Name: "<file...>", Description: "Audio paths to inspect"}}},
	"tag-track artwork embed":        {Args: []cobrahelptree.ArgSpec{{Name: "<file...>", Description: "Audio paths to embed artwork into"}}},
	"tag-track dependencies install": {Args: []cobrahelptree.ArgSpec{{Name: "[dep...]", Description: "ffmpeg or ffprobe; omitted selects all"}}},
	"tag-track dependencies update":  {Args: []cobrahelptree.ArgSpec{{Name: "[dep...]", Description: "ffmpeg or ffprobe; omitted selects all"}}},
}

func setupHelp(cmd *cobra.Command) {
	if err := cobrahelptree.SetupWithOptions(cmd, cobrahelptree.HelpOptions{
		Catalog: techCatalog,
		Tree:    cobrahelptree.TreeOptions{HideGeneratedCommands: true},
		Skill:   skillCommandOutput,
	}); err != nil {
		panic(err)
	} // Fixed options are a programmer invariant.
}
