package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/resolver"
	"github.com/spf13/cobra"
)

var pinCmd = &cobra.Command{
	Use:   "pin",
	Short: "Pin FROM images to their digests",
	Long:  "Parse Dockerfile FROM lines and add @sha256:<digest> to each image reference.",
	RunE:  runPin,
}

var (
	pinFilePath string
	pinGlob     string
	pinDryRun   bool
	pinUpdate   bool
	pinPlatform string
)

func init() {
	pinCmd.Flags().StringVarP(&pinFilePath, "file", "f", "", "Dockerfile path (default: ./Dockerfile)")
	pinCmd.Flags().StringVar(&pinGlob, "glob", "", "Glob pattern to find Dockerfiles")
	pinCmd.Flags().BoolVar(&pinDryRun, "dry-run", false, "Show changes without writing files")
	pinCmd.Flags().BoolVar(&pinUpdate, "update", false, "Update existing digests")
	pinCmd.Flags().StringVar(&pinPlatform, "platform", "", "Platform for multi-arch images (e.g., linux/amd64)")
	rootCmd.AddCommand(pinCmd)
}

func runPin(cmd *cobra.Command, args []string) error {
	files, err := FindFiles(pinFilePath, pinGlob)
	if err != nil {
		return err
	}

	ctx := context.Background()
	res := &resolver.CraneResolver{}

	for _, filePath := range files {
		if err := pinDockerfile(ctx, filePath, res, pinDryRun, pinUpdate); err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filePath, err)
		}
	}
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
