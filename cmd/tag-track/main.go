package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/dj/tag-track-cli/internal/cache"
	"github.com/dj/tag-track-cli/internal/deps"
	"github.com/dj/tag-track-cli/internal/metadata"
	"github.com/dj/tag-track-cli/internal/progress"
	"github.com/dj/tag-track-cli/internal/spinner"
	"github.com/spf13/cobra"
)

var (
	version    = "dev"
	jsonOutput bool

	outDir         string
	inPlace        bool
	artistFlag     string
	titleFlag      string
	albumFlag      string
	dateFlag       string
	genreFlag      string
	commentFlag    string
	artworkFlag    string
	sourceURLFlag  string
	refreshFlag    bool
	dryRunFlag     bool
	noCache        bool
	verbose        bool
	progressTarget string
	progressSocket string
	autoInstall    bool

	stdinReader = io.Reader(os.Stdin)
)

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "tag-track [file...]",
		Short:        "Inspect, enrich, and embed normalized metadata and artwork on audio files",
		Version:      version,
		SilenceUsage: true,
		Long: `tag-track is a CLI tool for identifying, tagging, retagging, and embedding normalized artwork into DJ audio files with zero lossy transcoding.

When audio files are provided, tag-track identifies track metadata through a multi-tier fallback chain:
  - Acoustic identification (Shazam via goshazam / AcoustID via gochromaprint)
  - iTunes Search API (with 1400x1400 high-res square cover art)
  - MusicBrainz API + Cover Art Archive
  - Clean filename fallback

Tags and artwork are embedded with byte-for-byte stream preservation (-c:a copy) and origin release date preservation.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveInputFiles(args)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return cmd.Help()
			}

			if err := ensureDependencies(cmd.Context()); err != nil {
				return err
			}

			return processFiles(cmd.Context(), files, changedTagFlags(cmd))
		},
	}

	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output structured results as JSON")
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.Flags().StringVarP(&outDir, "out-dir", "o", "", "Output directory for tagged audio tracks (default: overwrite in-place or current directory)")
	rootCmd.Flags().BoolVarP(&inPlace, "in-place", "i", false, "Update audio files in-place")
	rootCmd.Flags().StringVar(&artistFlag, "artist", "", "Explicit artist tag override")
	rootCmd.Flags().StringVar(&titleFlag, "title", "", "Explicit title tag override")
	rootCmd.Flags().StringVar(&albumFlag, "album", "", "Explicit album tag override")
	rootCmd.Flags().StringVar(&dateFlag, "date", "", "Explicit release date/year tag override (YYYY or YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&genreFlag, "genre", "", "Explicit genre tag override")
	rootCmd.Flags().StringVar(&commentFlag, "comment", "", "Explicit comment tag override")
	rootCmd.Flags().StringVar(&sourceURLFlag, "source-url", "", "Source URL of the audio stream to record in provenance tags")
	rootCmd.Flags().StringVar(&artworkFlag, "artwork", "", "Cover artwork image URL or local file path to embed")
	rootCmd.Flags().StringVar(&artworkFlag, "cover", "", "Alias for --artwork")
	_ = rootCmd.Flags().MarkHidden("cover")
	rootCmd.Flags().BoolVar(&refreshFlag, "refresh", false, "Force re-identification and re-tagging of files")
	rootCmd.Flags().BoolVar(&refreshFlag, "force", false, "Alias for --refresh")
	_ = rootCmd.Flags().MarkHidden("force")
	rootCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview proposed metadata without modifying files on disk")
	rootCmd.Flags().BoolVar(&noCache, "no-cache", false, "Disable local caching for search queries, metadata, and artwork")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output logging")
	rootCmd.Flags().StringVar(&progressTarget, "progress-target", "", "Target URI/address for streaming JSON progress events (e.g. unix:///path/to.sock, tcp://127.0.0.1:9099, fd://3, stdout, stderr)")
	rootCmd.Flags().StringVar(&progressSocket, "progress-socket", "", "Shorthand alias for --progress-target")
	_ = rootCmd.Flags().MarkHidden("progress-socket")
	rootCmd.Flags().BoolVar(&autoInstall, "auto-install", false, "Automatically install missing dependencies without prompting")

	// Subject: track
	trackCmd := &cobra.Command{
		Use:   "track",
		Short: "Inspect and update audio track tags and metadata",
	}

	trackUpdateCmd := &cobra.Command{
		Use:          "update [file...]",
		Short:        "Identify and update metadata and artwork on audio files",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveInputFiles(args)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return cmd.Help()
			}
			if err := ensureDependencies(cmd.Context()); err != nil {
				return err
			}
			return processFiles(cmd.Context(), files, changedTagFlags(cmd))
		},
	}
	copyFlags(rootCmd, trackUpdateCmd)

	trackInspectCmd := &cobra.Command{
		Use:          "inspect <file...>",
		Short:        "Display existing metadata tags, audio codecs, and artwork properties",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveInputFiles(args)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return cmd.Help()
			}
			if err := ensureDependencies(cmd.Context()); err != nil {
				return err
			}
			return inspectFiles(cmd.Context(), files)
		},
	}

	trackCmd.AddCommand(trackUpdateCmd, trackInspectCmd)

	// Top-level aliases for subject commands
	rootCmd.AddCommand(trackCmd)

	topUpdateCmd := &cobra.Command{
		Use:          "update [file...]",
		Short:        "Identify and update metadata and artwork on audio files (alias for 'track update')",
		SilenceUsage: true,
		RunE:         trackUpdateCmd.RunE,
	}
	copyFlags(rootCmd, topUpdateCmd)
	rootCmd.AddCommand(topUpdateCmd)

	topInspectCmd := &cobra.Command{
		Use:          "inspect <file...>",
		Short:        "Display existing metadata tags, audio codecs, and artwork properties (alias for 'track inspect')",
		SilenceUsage: true,
		RunE:         trackInspectCmd.RunE,
	}
	rootCmd.AddCommand(topInspectCmd)

	// Subject: artwork
	artworkCmd := &cobra.Command{
		Use:   "artwork",
		Short: "Manage and embed album cover artwork",
	}

	artworkEmbedCmd := &cobra.Command{
		Use:          "embed <file...>",
		Short:        "Embed 1400x1400 normalized square cover art into audio files",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveInputFiles(args)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return cmd.Help()
			}
			if artworkFlag == "" {
				return errors.New("must specify artwork with --artwork <url|path> or --url <url>")
			}
			if err := ensureDependencies(cmd.Context()); err != nil {
				return err
			}
			return embedArtworkOnFiles(cmd.Context(), files, artworkFlag)
		},
	}
	artworkEmbedCmd.Flags().StringVar(&artworkFlag, "url", "", "Artwork image URL to download and embed")
	artworkEmbedCmd.Flags().StringVar(&artworkFlag, "artwork", "", "Artwork image URL or local file path to embed")
	artworkEmbedCmd.Flags().StringVarP(&outDir, "out-dir", "o", "", "Output directory for tagged audio tracks")
	artworkEmbedCmd.Flags().BoolVarP(&inPlace, "in-place", "i", false, "Update audio files in-place")
	artworkCmd.AddCommand(artworkEmbedCmd)
	rootCmd.AddCommand(artworkCmd)

	// Dependencies command
	depsCmd := &cobra.Command{
		Use:          "dependencies",
		Aliases:      []string{"deps"},
		Short:        "Verify required external binary dependencies (ffmpeg, ffprobe)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput {
				return runDepsJSON(cmd, args, "inspect")
			}
			isAgent := deps.IsAgentMode()
			reports, err := deps.VerifyDependencies(cmd.Context())

			if isAgent {
				for _, r := range reports {
					if r.Satisfied {
						fmt.Printf("%s: ok (version %s, min %s)\n", r.Name, r.DetectedVersion, r.MinVersion)
					} else if !r.Installed {
						fmt.Printf("%s: missing\n", r.Name)
					} else {
						fmt.Printf("%s: fail (version %s, min %s)\n", r.Name, r.DetectedVersion, r.MinVersion)
					}
				}
				if err != nil {
					fmt.Printf("status: error\nerror: %v\n", err)
					return err
				}
				fmt.Println("status: ok")
				return nil
			}

			for _, r := range reports {
				if r.Satisfied {
					fmt.Printf("%s: %s (min %s) [OK]\n", r.Name, r.DetectedVersion, r.MinVersion)
				} else if !r.Installed {
					fmt.Printf("%s: missing in $PATH [FAIL] - %s\n", r.Name, r.Error)
				} else {
					fmt.Printf("%s: %s (min %s) [FAIL] - %s\n", r.Name, r.DetectedVersion, r.MinVersion, r.Error)
				}
			}

			if err != nil {
				return err
			}
			fmt.Println("\nAll dependencies met.")
			return nil
		},
	}

	depsInstallCmd := &cobra.Command{
		Use:          "install [dep...]",
		Aliases:      []string{"add", "get"},
		Short:        "Install missing external dependencies (ffmpeg, ffprobe)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput {
				return runDepsJSON(cmd, args, "install")
			}
			_ = deps.InitManagedPath()
			if len(args) > 0 {
				for _, depName := range args {
					fmt.Printf("Installing %s...\n", depName)
					if err := deps.InstallDependency(cmd.Context(), depName); err != nil {
						return fmt.Errorf("installing %s: %w", depName, err)
					}
					fmt.Printf("%s installed successfully.\n", depName)
				}
				return nil
			}

			fmt.Println("Checking and installing missing dependencies...")
			installed, err := deps.InstallMissingDependencies(cmd.Context())
			if err != nil {
				return err
			}
			if len(installed) == 0 {
				fmt.Println("All dependencies are already satisfied.")
			} else {
				fmt.Printf("Successfully installed: %s\n", strings.Join(installed, ", "))
			}
			return nil
		},
	}

	depsUpdateCmd := &cobra.Command{
		Use:          "update [dep...]",
		Aliases:      []string{"upgrade"},
		Short:        "Update external dependencies to their latest versions",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput {
				return runDepsJSON(cmd, args, "update")
			}
			_ = deps.InitManagedPath()
			if len(args) > 0 {
				for _, depName := range args {
					fmt.Printf("Updating %s...\n", depName)
					if err := deps.UpdateDependency(cmd.Context(), depName); err != nil {
						return fmt.Errorf("updating %s: %w", depName, err)
					}
					fmt.Printf("%s updated successfully.\n", depName)
				}
				return nil
			}

			fmt.Println("Updating all dependencies to latest versions...")
			updated, err := deps.UpdateAllDependencies(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("Successfully updated: %s\n", strings.Join(updated, ", "))
			return nil
		},
	}

	depsCmd.AddCommand(depsInstallCmd, depsUpdateCmd)
	rootCmd.AddCommand(depsCmd)

	// Upgrade command
	upgradeCmd := &cobra.Command{
		Use:          "upgrade",
		Aliases:      []string{"self-update", "update-self"},
		Short:        "Upgrade tag-track CLI binary to the latest released version",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput {
				return runUpgradeJSON(cmd, version)
			}
			isAgent := deps.IsAgentMode()
			if !isAgent {
				fmt.Printf("Checking for newer tag-track release (current version: %s)...\n", version)
			}
			newVer, err := deps.UpgradeSelf(cmd.Context(), version)
			if err != nil {
				if strings.Contains(err.Error(), "already at the latest version") {
					if isAgent {
						fmt.Printf("upgrade: current\nversion: %s\n", version)
					} else {
						fmt.Printf("tag-track is already up to date (version %s).\n", version)
					}
					return nil
				}
				return fmt.Errorf("upgrade failed: %w", err)
			}
			if isAgent {
				fmt.Printf("upgrade: ok\nversion: %s\n", newVer)
			} else {
				fmt.Printf("Successfully upgraded tag-track to version %s!\n", newVer)
			}
			return nil
		},
	}
	rootCmd.AddCommand(upgradeCmd)

	cobrahelptree.Setup(rootCmd)
	return rootCmd
}

func copyFlags(src, dst *cobra.Command) {
	dst.Flags().StringVarP(&outDir, "out-dir", "o", "", "Output directory for tagged audio tracks (default: overwrite in-place or current directory)")
	dst.Flags().BoolVarP(&inPlace, "in-place", "i", false, "Update audio files in-place")
	dst.Flags().StringVar(&artistFlag, "artist", "", "Explicit artist tag override")
	dst.Flags().StringVar(&titleFlag, "title", "", "Explicit title tag override")
	dst.Flags().StringVar(&albumFlag, "album", "", "Explicit album tag override")
	dst.Flags().StringVar(&dateFlag, "date", "", "Explicit release date/year tag override (YYYY or YYYY-MM-DD)")
	dst.Flags().StringVar(&genreFlag, "genre", "", "Explicit genre tag override")
	dst.Flags().StringVar(&commentFlag, "comment", "", "Explicit comment tag override")
	dst.Flags().StringVar(&sourceURLFlag, "source-url", "", "Source URL of the audio stream to record in provenance tags")
	dst.Flags().StringVar(&artworkFlag, "artwork", "", "Cover artwork image URL or local file path to embed")
	dst.Flags().StringVar(&artworkFlag, "cover", "", "Alias for --artwork")
	_ = dst.Flags().MarkHidden("cover")
	dst.Flags().BoolVar(&refreshFlag, "refresh", false, "Force re-identification and re-tagging of files")
	dst.Flags().BoolVar(&refreshFlag, "force", false, "Alias for --refresh")
	_ = dst.Flags().MarkHidden("force")
	dst.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview proposed metadata without modifying files on disk")
	dst.Flags().BoolVar(&noCache, "no-cache", false, "Disable local caching for search queries, metadata, and artwork")
	dst.Flags().StringVar(&progressTarget, "progress-target", "", "Target URI/address for streaming JSON progress events")
	dst.Flags().StringVar(&progressSocket, "progress-socket", "", "Shorthand alias for --progress-target")
	_ = dst.Flags().MarkHidden("progress-socket")
	dst.Flags().BoolVar(&autoInstall, "auto-install", false, "Automatically install missing dependencies without prompting")
}

func resolveInputFiles(args []string) ([]string, error) {
	// If "-" or no args and stdin is not a terminal, read from stdin
	if len(args) == 0 || (len(args) == 1 && args[0] == "-") {
		stat, _ := os.Stdin.Stat()
		if (stat != nil && (stat.Mode()&os.ModeCharDevice) == 0) || (len(args) == 1 && args[0] == "-") {
			var allLines []string
			var prefixedFiles []string
			scanner := bufio.NewScanner(stdinReader)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				if strings.HasPrefix(line, "DONE: ") {
					prefixedFiles = append(prefixedFiles, strings.TrimSpace(strings.TrimPrefix(line, "DONE: ")))
				} else if strings.HasPrefix(line, "output: ") {
					prefixedFiles = append(prefixedFiles, strings.TrimSpace(strings.TrimPrefix(line, "output: ")))
				} else {
					allLines = append(allLines, line)
				}
			}
			if len(prefixedFiles) > 0 {
				return prefixedFiles, scanner.Err()
			}
			return allLines, scanner.Err()
		}
	}

	var files []string
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed != "" && trimmed != "-" {
			files = append(files, trimmed)
		}
	}

	return files, nil
}

func ensureDependencies(ctx context.Context) error {
	_ = deps.InitManagedPath()
	c, _ := cache.New(!noCache)
	reports, err := deps.VerifyDependencies(ctx, c)
	if err == nil {
		return nil
	}

	var missing []string
	var missingNames []string
	for _, r := range reports {
		if !r.Satisfied {
			missingNames = append(missingNames, r.Name)
			if r.Installed && r.DetectedVersion != "" {
				missing = append(missing, fmt.Sprintf("%s (installed version %s, required %s or newer)", r.Name, r.DetectedVersion, r.MinVersion))
			} else {
				missing = append(missing, fmt.Sprintf("%s (missing, required %s or newer)", r.Name, r.MinVersion))
			}
		}
	}

	if autoInstall {
		if !deps.IsAgentMode() && !jsonOutput {
			fmt.Printf("Auto-installing missing dependencies: %s...\n", strings.Join(missing, ", "))
		}
		installed, installErr := deps.InstallMissingDependencies(ctx)
		if installErr != nil {
			return fmt.Errorf("auto-installing dependencies: %w", installErr)
		}
		if len(installed) > 0 && !deps.IsAgentMode() && !jsonOutput {
			fmt.Printf("Successfully installed: %s\n", strings.Join(installed, ", "))
		}
		return nil
	}

	if !deps.IsAgentMode() && !jsonOutput {
		reader := bufio.NewReader(stdinReader)
		fmt.Printf("\nMissing required dependencies: %s\nWould you like to auto-install them to managed directory? [Y/n]: ", strings.Join(missing, ", "))
		ans, _ := reader.ReadString('\n')
		ans = strings.TrimSpace(strings.ToLower(ans))
		if ans == "" || ans == "y" || ans == "yes" {
			fmt.Printf("Installing dependencies: %s...\n", strings.Join(missingNames, ", "))
			installed, installErr := deps.InstallMissingDependencies(ctx)
			if installErr != nil {
				return fmt.Errorf("auto-installing dependencies: %w", installErr)
			}
			if len(installed) > 0 {
				fmt.Printf("Successfully installed: %s\n\n", strings.Join(installed, ", "))
			}
			return nil
		}
	}

	return err
}

func inspectFiles(ctx context.Context, files []string) error {
	batch := tagBatch{Status: "success", Results: []tagResult{}}
	isAgent := deps.IsAgentMode()

	for _, file := range files {
		info, err := metadata.InspectAudioTrack(ctx, file, nil)
		if jsonOutput {
			r := tagResult{InputFile: file, Status: "success", Inspection: info}
			if err != nil {
				r.Status = "error"
				r.Error = err.Error()
			}
			batch.Results = append(batch.Results, r)
			continue
		}
		if err != nil {
			if isAgent {
				fmt.Printf("file: %s\nstatus: error\nerror: %v\n", file, err)
			} else {
				fmt.Printf("[ERROR] Inspecting %s: %v\n", file, err)
			}
			continue
		}

		if isAgent {
			fmt.Printf("file: %s\n", info.Path)
			fmt.Printf("format: %s\n", info.Format)
			fmt.Printf("codec: %s\n", info.Codec)
			fmt.Printf("duration: %s\n", info.Duration.Round(time.Second))
			fmt.Printf("bitrate: %d kbps\n", info.BitRateKbps)
			fmt.Printf("sample_rate: %d Hz\n", info.SampleRateHz)
			fmt.Printf("channels: %d\n", info.Channels)
			if info.Title != "" {
				fmt.Printf("title: %s\n", info.Title)
			}
			if info.Artist != "" {
				fmt.Printf("artist: %s\n", info.Artist)
			}
			if info.Album != "" {
				fmt.Printf("album: %s\n", info.Album)
			}
			if info.Date != "" {
				fmt.Printf("date: %s\n", info.Date)
			}
			if info.Genre != "" {
				fmt.Printf("genre: %s\n", info.Genre)
			}
			if info.Comment != "" {
				fmt.Printf("comment: %s\n", info.Comment)
			}
			fmt.Printf("has_artwork: %t\n", info.HasArtwork)
			if info.HasArtwork {
				fmt.Printf("artwork_dimensions: %dx%d\n", info.ArtworkWidth, info.ArtworkHeight)
				fmt.Printf("artwork_square: %t\n", info.IsSquareArtwork)
			}
		} else {
			fmt.Printf("File: %s\n", info.Path)
			fmt.Printf("  Codec / Format: %s (%s, %d kbps, %d Hz)\n", info.Codec, info.Format, info.BitRateKbps, info.SampleRateHz)
			fmt.Printf("  Duration: %s\n", info.Duration.Round(time.Second))
			if info.Title != "" {
				fmt.Printf("  Title: %s\n", info.Title)
			}
			if info.Artist != "" {
				fmt.Printf("  Artist: %s\n", info.Artist)
			}
			if info.Album != "" {
				fmt.Printf("  Album: %s\n", info.Album)
			}
			if info.Date != "" {
				fmt.Printf("  Date / Year: %s\n", info.Date)
			}
			if info.Genre != "" {
				fmt.Printf("  Genre: %s\n", info.Genre)
			}
			if info.Comment != "" {
				fmt.Printf("  Comment: %s\n", info.Comment)
			}
			if info.HasArtwork {
				squareNote := ""
				if info.IsSquareArtwork {
					squareNote = " (1:1 square)"
				}
				fmt.Printf("  Cover Art: %dx%d%s\n", info.ArtworkWidth, info.ArtworkHeight, squareNote)
			} else {
				fmt.Printf("  Cover Art: none\n")
			}
			fmt.Println()
		}
	}

	if jsonOutput {
		return batch.write()
	}
	return nil
}

func embedArtworkOnFiles(ctx context.Context, files []string, artURL string) error {
	batch := tagBatch{Status: "success", Results: []tagResult{}}
	isAgent := deps.IsAgentMode() || jsonOutput
	cacheInst, _ := cache.New(!noCache)

	for _, file := range files {
		targetOutDir := outDir
		if targetOutDir == "" || inPlace {
			targetOutDir = filepath.Dir(file)
		}

		meta := metadata.TrackMetadataResult{
			CoverArtURL:    artURL,
			RequireArtwork: true,
			Source:         "Direct Artwork",
		}

		// Read existing tags if present so they aren't lost
		if existing, err := metadata.InspectAudioTrack(ctx, file, nil); err == nil && existing != nil {
			meta.Title = existing.Title
			meta.Artist = existing.Artist
			meta.Album = existing.Album
			meta.ReleaseDate = existing.Date
			meta.ReleaseYear = existing.Date
			meta.Genre = existing.Genre
			meta.Comment = &existing.Comment
		}

		if !isAgent {
			fmt.Printf("Embedding artwork into %s...\n", file)
		}

		outPath, err := metadata.ApplyMetadataToLocalTrack(ctx, file, meta, targetOutDir, verbose, cacheInst)
		if err != nil {
			if jsonOutput {
				batch.Results = append(batch.Results, tagResult{InputFile: file, Status: "error", Error: err.Error()})
				continue
			}
			if isAgent {
				fmt.Printf("file: %s\nstatus: error\nerror: %v\n", file, err)
			} else {
				fmt.Printf("[ERROR] Embedding artwork on %s: %v\n", file, err)
			}
			continue
		}

		if inPlace && outPath != file {
			_ = os.Remove(file)
		}

		if jsonOutput {
			batch.Results = append(batch.Results, tagResult{InputFile: file, OutputFile: outPath, Status: "success", Metadata: &meta})
			continue
		}
		if isAgent {
			fmt.Printf("file: %s\nartwork: %s\nstatus: ok\noutput: %s\n", file, artURL, outPath)
		} else {
			fmt.Printf("[OK] Artwork embedded: %s\n", outPath)
		}
	}

	if jsonOutput {
		return batch.write()
	}
	return nil
}

func changedTagFlags(cmd *cobra.Command) map[string]bool {
	changed := make(map[string]bool)
	for _, name := range []string{"artist", "title", "album", "date", "genre", "comment", "artwork", "source-url"} {
		if cmd.Flags().Changed(name) {
			changed[name] = true
		}
	}
	return changed
}

func processFiles(ctx context.Context, files []string, changed map[string]bool) error {
	batch := tagBatch{Status: "success", Results: []tagResult{}}
	isAgent := deps.IsAgentMode() || jsonOutput
	cacheInst, _ := cache.New(!noCache && !refreshFlag)
	client := metadata.NewClient(cacheInst)

	resolvedTarget := progress.ResolveTargetURI(progressTarget, progressSocket, os.Getenv("TAG_TRACK_PROGRESS_TARGET"))
	var rep *progress.Reporter
	if resolvedTarget != "" {
		r, err := progress.NewReporter(ctx, resolvedTarget)
		if err == nil {
			rep = r
			defer rep.Close()
		}
	}

	for i, file := range files {
		if stat, err := os.Stat(file); err != nil || stat.IsDir() {
			if jsonOutput {
				batch.Results = append(batch.Results, tagResult{InputFile: file, Status: "error", Error: "file not found or is directory"})
				continue
			}
			if isAgent {
				fmt.Printf("file: %s\nstatus: error\nerror: file not found or is directory\n", file)
			} else {
				fmt.Printf("[ERROR] File not found or is directory: %s\n", file)
			}
			continue
		}

		if rep != nil {
			_ = rep.Emit(progress.Event{
				Type:       progress.EventPhaseStart,
				Phase:      "identify",
				Step:       i + 1,
				TotalSteps: len(files),
				Message:    fmt.Sprintf("identifying track metadata for %s", filepath.Base(file)),
			})
		}

		var sp *spinner.Spinner
		if !isAgent && !verbose {
			sp = spinner.New(fmt.Sprintf("working... identifying %s", filepath.Base(file)))
			sp.Start()
		}

		baseName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		cleanSearch := metadata.SanitizeFilename(baseName)

		rawArtist := artistFlag
		rawTitle := titleFlag
		if rawArtist == "" && rawTitle == "" && strings.Contains(cleanSearch, " - ") {
			parts := strings.SplitN(cleanSearch, " - ", 2)
			rawArtist = strings.TrimSpace(parts[0])
			rawTitle = strings.TrimSpace(parts[1])
		}

		metaRes, err := client.ResolveTrackMetadataFallback(ctx, file, cleanSearch, rawArtist, rawTitle, verbose)
		if sp != nil {
			sp.Stop()
		}

		if err != nil || metaRes == nil {
			metaRes = &metadata.TrackMetadataResult{
				Title:  cleanSearch,
				Artist: "Unknown Artist",
				Source: "Filename Fallback",
			}
		}

		// Apply manual overrides if specified via flags
		if changed["artist"] {
			metaRes.Artist = artistFlag
		}
		if changed["title"] {
			metaRes.Title = titleFlag
		}
		if changed["album"] {
			metaRes.Album = albumFlag
		}
		if changed["date"] {
			metaRes.ReleaseDate = dateFlag
			metaRes.ReleaseYear = ""
			if len(dateFlag) >= 4 {
				metaRes.ReleaseYear = dateFlag[:4]
			}
		}
		if changed["genre"] {
			metaRes.Genre = genreFlag
		}
		if changed["comment"] {
			metaRes.Comment = &commentFlag
		}
		if artworkFlag != "" {
			metaRes.CoverArtURL = artworkFlag
			metaRes.RequireArtwork = true
		}
		if sourceURLFlag != "" {
			metaRes.AudioSourceURL = sourceURLFlag
		}

		if rep != nil {
			_ = rep.Emit(progress.Event{
				Type:       progress.EventMetadataFound,
				Phase:      "identify",
				Step:       i + 1,
				TotalSteps: len(files),
				Metadata: &progress.MetadataInfo{
					Title:       metaRes.Title,
					Artist:      metaRes.Artist,
					Album:       metaRes.Album,
					Genre:       metaRes.Genre,
					ReleaseDate: metaRes.ReleaseDate,
					ReleaseYear: metaRes.ReleaseYear,
					CoverArtURL: metaRes.CoverArtURL,
					Source:      metaRes.Source,
				},
			})
		}

		if dryRunFlag {
			if jsonOutput {
				batch.Results = append(batch.Results, tagResult{InputFile: file, Status: "success", Metadata: metaRes, DryRun: true})
				continue
			}
			if isAgent {
				fmt.Printf("file: %s\ntitle: %s\nartist: %s\nalbum: %s\nyear: %s\nsource: %s\ncover_art: %s\ndry_run: true\n",
					file, metaRes.Title, metaRes.Artist, metaRes.Album, metaRes.ReleaseYear, metaRes.Source, metaRes.CoverArtURL)
			} else {
				fmt.Printf("Preview for %s:\n", file)
				fmt.Printf("  Artist: %s\n", metaRes.Artist)
				fmt.Printf("  Title: %s\n", metaRes.Title)
				if metaRes.Album != "" {
					fmt.Printf("  Album: %s\n", metaRes.Album)
				}
				if metaRes.ReleaseYear != "" {
					fmt.Printf("  Year: %s\n", metaRes.ReleaseYear)
				}
				if metaRes.CoverArtURL != "" {
					fmt.Printf("  Artwork: %s\n", metaRes.CoverArtURL)
				}
				fmt.Printf("  Source: %s\n", metaRes.Source)
				fmt.Println("  [DRY RUN - file was not modified]")
				fmt.Println()
			}
			continue
		}

		targetOutDir := outDir
		if targetOutDir == "" || inPlace {
			targetOutDir = filepath.Dir(file)
		}

		taggedPath, err := metadata.ApplyMetadataToLocalTrack(ctx, file, *metaRes, targetOutDir, verbose, cacheInst)
		if err != nil {
			if jsonOutput {
				batch.Results = append(batch.Results, tagResult{InputFile: file, Status: "error", Error: err.Error()})
				continue
			}
			if rep != nil {
				_ = rep.Emit(progress.Event{
					Type:  progress.EventError,
					Error: err.Error(),
				})
			}
			if isAgent {
				fmt.Printf("file: %s\nstatus: error\nerror: %v\n", file, err)
			} else {
				fmt.Printf("[ERROR] Tagging %s: %v\n", file, err)
			}
			continue
		}

		if inPlace && taggedPath != file {
			_ = os.Remove(file)
		}

		if rep != nil {
			_ = rep.Emit(progress.Event{
				Type:       progress.EventComplete,
				Phase:      "complete",
				Step:       i + 1,
				TotalSteps: len(files),
				Result: &progress.ResultInfo{
					Path:        taggedPath,
					Title:       metaRes.Title,
					Artist:      metaRes.Artist,
					Album:       metaRes.Album,
					ReleaseYear: metaRes.ReleaseYear,
					Source:      metaRes.Source,
					Status:      "ok",
				},
			})
		}

		if jsonOutput {
			batch.Results = append(batch.Results, tagResult{InputFile: file, OutputFile: taggedPath, Status: "success", Metadata: metaRes})
			continue
		}
		if isAgent {
			fmt.Printf("file: %s\n", file)
			fmt.Printf("title: %s\n", metaRes.Title)
			fmt.Printf("artist: %s\n", metaRes.Artist)
			if metaRes.Album != "" {
				fmt.Printf("album: %s\n", metaRes.Album)
			}
			if metaRes.ReleaseYear != "" {
				fmt.Printf("year: %s\n", metaRes.ReleaseYear)
			}
			fmt.Printf("source: %s\n", metaRes.Source)
			fmt.Printf("output: %s\n", taggedPath)
		} else {
			if metaRes.Title != "" {
				fmt.Printf("  - matched: \"%s - %s\" (%s, %s)\n", metaRes.Artist, metaRes.Title, metaRes.Album, metaRes.ReleaseYear)
				fmt.Printf("  - source: %s\n", metaRes.Source)
			}
			fmt.Printf("DONE: %s\n", taggedPath)
		}
	}

	if jsonOutput {
		return batch.write()
	}
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nForced termination requested. Exiting.")
		os.Exit(130)
	}()

	cmd := newRootCommand()
	if err := cmd.ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "Operation canceled by user.")
			os.Exit(130)
		}
		os.Exit(1)
	}
}

type tagResult struct {
	InputFile  string                        `json:"input_file"`
	OutputFile string                        `json:"output_file,omitempty"`
	Status     string                        `json:"status"`
	Error      string                        `json:"error,omitempty"`
	Inspection *metadata.TrackInspection     `json:"inspection,omitempty"`
	Metadata   *metadata.TrackMetadataResult `json:"metadata,omitempty"`
	DryRun     bool                          `json:"dry_run,omitempty"`
}
type tagBatch struct {
	Status  string      `json:"status"`
	Results []tagResult `json:"results"`
}

func (b tagBatch) write() error {
	var failures []error
	for _, r := range b.Results {
		if r.Error != "" {
			b.Status = "error"
			failures = append(failures, fmt.Errorf("%s: %s", r.InputFile, r.Error))
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(b); err != nil {
		return err
	}
	return errors.Join(failures...)
}
