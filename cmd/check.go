package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/azu/dockerfile-pin/internal/compose"
	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/resolver"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if FROM images are pinned to digests",
	Long:  "Validate that Dockerfile FROM lines have @sha256:<digest> and that digests exist in the registry.",
	Args:  cobra.NoArgs,
	RunE:  runCheck,
}

var (
	checkFilePath   string
	checkGlob       string
	checkSyntaxOnly bool
	checkFormat     string
	checkIgnore     []string
	checkExitCode   int
)

func init() {
	checkCmd.Flags().StringVarP(&checkFilePath, "file", "f", "", "Dockerfile path (default: ./Dockerfile)")
	checkCmd.Flags().StringVar(&checkGlob, "glob", "", "Glob pattern to find Dockerfiles")
	checkCmd.Flags().BoolVar(&checkSyntaxOnly, "syntax-only", false, "Skip registry checks")
	checkCmd.Flags().StringVar(&checkFormat, "format", "text", "Output format: text or json")
	checkCmd.Flags().StringSliceVar(&checkIgnore, "ignore-images", nil, "Images to ignore")
	checkCmd.Flags().IntVar(&checkExitCode, "exit-code", 1, "Exit code on failure")
	rootCmd.AddCommand(checkCmd)
}

type CheckResult struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Image    string `json:"image"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Original string `json:"original"`
}

func runCheck(cmd *cobra.Command, args []string) error {
	files, err := FindFiles(checkFilePath, checkGlob)
	if err != nil {
		return err
	}

	ctx := context.Background()
	res := &resolver.CraneResolver{}
	var results []CheckResult
	hasFail := false

	for _, filePath := range files {
		var fileResults []CheckResult
		var err error
		switch DetectFileType(filePath) {
		case FileTypeCompose:
			fileResults, err = checkComposeFile(ctx, filePath, res, checkSyntaxOnly, checkIgnore)
		default:
			fileResults, err = checkDockerfile(ctx, filePath, res, checkSyntaxOnly, checkIgnore)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filePath, err)
			continue
		}
		results = append(results, fileResults...)
		for _, r := range fileResults {
			if r.Status == "fail" {
				hasFail = true
			}
		}
	}

	switch checkFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
	default:
		for _, r := range results {
			var prefix string
			switch r.Status {
			case "ok":
				prefix = "OK   "
			case "fail":
				prefix = "FAIL "
			case "skip":
				prefix = "SKIP "
			case "warn":
				prefix = "WARN "
			}
			fmt.Printf("%-5s %s:%-4d %-50s %s\n", prefix, r.File, r.Line, r.Original, r.Message)
		}
	}

	if hasFail {
		os.Exit(checkExitCode)
	}
	return nil
}

func checkDockerfile(ctx context.Context, filePath string, res resolver.DigestResolver, syntaxOnly bool, ignoreImages []string) ([]CheckResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}

	instructions, err := dockerfile.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}

	var results []CheckResult
	for _, inst := range instructions {
		if inst.Skip {
			results = append(results, CheckResult{
				File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
				Status: "skip", Message: inst.SkipReason, Original: inst.Original,
			})
			continue
		}
		if isIgnored(inst.ImageRef, ignoreImages) {
			results = append(results, CheckResult{
				File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
				Status: "skip", Message: "ignored", Original: inst.Original,
			})
			continue
		}
		if inst.Digest == "" {
			results = append(results, CheckResult{
				File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
				Status: "fail", Message: "missing digest", Original: inst.Original,
			})
			continue
		}
		if syntaxOnly {
			results = append(results, CheckResult{
				File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
				Status: "ok", Message: "", Original: inst.Original,
			})
			continue
		}
		fullRef := inst.ImageRef + "@" + inst.Digest
		exists, err := res.Exists(ctx, fullRef)
		if err != nil {
			results = append(results, CheckResult{
				File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
				Status: "warn", Message: fmt.Sprintf("registry check failed: %v", err), Original: inst.Original,
			})
			continue
		}
		if !exists {
			results = append(results, CheckResult{
				File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
				Status: "fail", Message: "digest not found in registry", Original: inst.Original,
			})
			continue
		}
		results = append(results, CheckResult{
			File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
			Status: "ok", Message: "", Original: inst.Original,
		})
	}
	return results, nil
}

func checkComposeFile(ctx context.Context, filePath string, res resolver.DigestResolver, syntaxOnly bool, ignoreImages []string) ([]CheckResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}
	refs, err := compose.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	var results []CheckResult
	for _, ref := range refs {
		if ref.Skip {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "skip", Message: ref.SkipReason, Original: "image: " + ref.RawRef,
			})
			continue
		}
		if isIgnored(ref.ImageRef, ignoreImages) {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "skip", Message: "ignored", Original: "image: " + ref.RawRef,
			})
			continue
		}
		if ref.Digest == "" {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "fail", Message: "missing digest", Original: "image: " + ref.RawRef,
			})
			continue
		}
		if syntaxOnly {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "ok", Message: "", Original: "image: " + ref.RawRef,
			})
			continue
		}
		fullRef := ref.ImageRef + "@" + ref.Digest
		exists, err := res.Exists(ctx, fullRef)
		if err != nil {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "warn", Message: fmt.Sprintf("registry check failed: %v", err), Original: "image: " + ref.RawRef,
			})
			continue
		}
		if !exists {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "fail", Message: "digest not found in registry", Original: "image: " + ref.RawRef,
			})
			continue
		}
		results = append(results, CheckResult{
			File: filePath, Line: ref.Line, Image: ref.ImageRef,
			Status: "ok", Message: "", Original: "image: " + ref.RawRef,
		})
	}
	return results, nil
}

func isIgnored(imageRef string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(imageRef, pattern) {
			return true
		}
	}
	return false
}
