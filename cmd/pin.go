package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/azu/dockerfile-pin/internal/compose"
	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/resolver"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Pin FROM images to their digests",
	Long:  "Parse Dockerfile FROM lines and add @sha256:<digest> to each image reference.\nBy default, shows changes without writing files (dry-run). Use --write to apply changes.",
	RunE:  runRun,
}

var (
	runFilePath string
	runGlob     string
	runWrite    bool
	runUpdate   bool
	runPlatform string
)

func init() {
	runCmd.Flags().StringVarP(&runFilePath, "file", "f", "", "Dockerfile path (default: auto-detect)")
	runCmd.Flags().StringVar(&runGlob, "glob", "", "Glob pattern to find Dockerfiles")
	runCmd.Flags().BoolVar(&runWrite, "write", false, "Write changes to files (default is dry-run)")
	runCmd.Flags().BoolVar(&runUpdate, "update", false, "Update existing digests")
	runCmd.Flags().StringVar(&runPlatform, "platform", "", "Platform for multi-arch images (e.g., linux/amd64)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	files, err := FindFiles(runFilePath, runGlob)
	if err != nil {
		return err
	}

	dryRun := !runWrite
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res := &resolver.CraneResolver{}

	fmt.Printf("found %d file(s)\n", len(files))
	for i, filePath := range files {
		fmt.Printf("[%d/%d] %s\n", i+1, len(files), filePath)
		var err error
		switch DetectFileType(filePath) {
		case FileTypeCompose:
			err = pinComposeFile(ctx, filePath, res, dryRun, runUpdate)
		default:
			err = pinDockerfile(ctx, filePath, res, dryRun, runUpdate)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filePath, err)
		}
	}
	return nil
}

func pinComposeFile(ctx context.Context, filePath string, res resolver.DigestResolver, dryRun bool, update bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}
	refs, err := compose.Parse(content)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}
	digests := make(map[int]string)
	for i, ref := range refs {
		if ref.Skip {
			if ref.SkipReason != "" {
				fmt.Fprintf(os.Stderr, "SKIP  %s:%d  %s  %s\n", filePath, ref.Line, ref.RawRef, ref.SkipReason)
			}
			continue
		}
		if ref.Digest != "" && !update {
			continue
		}
		digest, err := res.Resolve(ctx, ref.ImageRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN  %s:%d  %s  failed to resolve: %v\n", filePath, ref.Line, ref.RawRef, err)
			continue
		}
		digests[i] = digest
	}
	if len(digests) == 0 {
		return nil
	}
	result := compose.RewriteFile(string(content), refs, digests)
	if dryRun {
		fmt.Printf("--- %s\n", filePath)
		fmt.Print(result)
		return nil
	}
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filePath, err)
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), filePath)
	return nil
}

func pinDockerfile(ctx context.Context, filePath string, res resolver.DigestResolver, dryRun bool, update bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	instructions, err := dockerfile.Parse(strings.NewReader(string(content)))
	if err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	digests := make(map[int]string)
	for i, inst := range instructions {
		if inst.Skip {
			if inst.SkipReason == "unresolved ARG variable" {
				fmt.Fprintf(os.Stderr, "WARN  %s:%d  %s  %s\n", filePath, inst.StartLine, inst.Original, inst.SkipReason)
			}
			continue
		}
		if inst.Digest != "" && !update {
			continue
		}
		digest, err := res.Resolve(ctx, inst.ImageRef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN  %s:%d  %s  failed to resolve: %v\n", filePath, inst.StartLine, inst.Original, err)
			continue
		}
		digests[i] = digest
	}

	if len(digests) == 0 {
		return nil
	}

	result := dockerfile.RewriteFile(string(content), instructions, digests)

	if dryRun {
		fmt.Printf("--- %s\n", filePath)
		fmt.Print(result)
		return nil
	}

	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filePath, err)
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), filePath)
	return nil
}
