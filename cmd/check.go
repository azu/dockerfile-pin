package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/azu/dockerfile-pin/internal/actions"
	"github.com/azu/dockerfile-pin/internal/compose"
	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/gitlab"
	"github.com/azu/dockerfile-pin/internal/resolver"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if FROM and COPY --from images are pinned to digests",
	Long: `Validate that every Docker image reference has a @sha256:<digest> and that the
digest exists in the registry.

Each image is reported as one of:
  OK     digest present and verified in registry
  FAIL   missing digest, or digest not found in registry
  SKIP   scratch, multi-stage ref, stage index, ignored, non-Docker uses, or CI variable
  WARN   registry check failed (network error, auth issue, etc.)

Exit code is 1 (configurable with --exit-code) when any image has FAIL status.
Use --syntax-only to skip registry verification (only checks digest presence).
Use --verify-tag to also check that each tag currently resolves to the pinned
digest. Tags are mutable, so this fails when the tag has moved since pinning.
Use --format json for machine-readable output (array of objects with
file, line, image, status, message, original fields).`,
	Example: `  # Check auto-detected files
  dockerfile-pin check

  # Check a specific file
  dockerfile-pin check -f Dockerfile

  # Check multiple files
  dockerfile-pin check --glob '**/{Dockerfile,docker-compose.yml}'

  # Syntax only (no network, fast)
  dockerfile-pin check --syntax-only

  # JSON output for CI integration
  dockerfile-pin check --format json

  # Verify that each tag still points to the pinned digest
  dockerfile-pin check --verify-tag

  # Ignore specific images
  dockerfile-pin check --ignore-images "scratch" --ignore-images "ghcr.io/myorg/*"`,
	Args: cobra.NoArgs,
	RunE: runCheck,
}

var (
	checkFilePath   string
	checkGlob       string
	checkSyntaxOnly bool
	checkFormat     string
	checkIgnore     []string
	checkExitCode   int
	checkVerifyTag  bool
)

func init() {
	checkCmd.Flags().StringVarP(&checkFilePath, "file", "f", "", "Dockerfile path (default: ./Dockerfile)")
	checkCmd.Flags().StringVar(&checkGlob, "glob", "", "Glob pattern to find Dockerfiles")
	checkCmd.Flags().BoolVar(&checkSyntaxOnly, "syntax-only", false, "Skip registry checks")
	checkCmd.Flags().StringVar(&checkFormat, "format", "text", "Output format: text or json")
	checkCmd.Flags().StringArrayVar(&checkIgnore, "ignore-images", nil, "Images to ignore (glob patterns, repeatable)")
	checkCmd.Flags().IntVar(&checkExitCode, "exit-code", 1, "Exit code on failure")
	checkCmd.Flags().BoolVar(&checkVerifyTag, "verify-tag", false, "Verify that each tag currently resolves to the pinned digest")
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
	if checkVerifyTag && checkSyntaxOnly {
		return fmt.Errorf("--verify-tag cannot be used with --syntax-only")
	}
	files, err := FindFiles(checkFilePath, checkGlob)
	if err != nil {
		return err
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	ignorePatterns := MergeIgnorePatterns(cfg.IgnoreImages, checkIgnore)
	if err := ValidatePatterns(ignorePatterns); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res := resolver.NewCachedResolver(&resolver.CraneResolver{})

	// Phase 1: parse all files and collect results (no registry calls yet)
	var allResults []CheckResult
	var needsCheck []int // indices into allResults that need registry verification
	for _, filePath := range files {
		var fileResults []CheckResult
		var err error
		switch DetectFileType(filePath) {
		case FileTypeCompose:
			fileResults, err = parseComposeForCheck(filePath, checkSyntaxOnly, ignorePatterns)
		case FileTypeActions:
			fileResults, err = parseActionsForCheck(filePath, checkSyntaxOnly, ignorePatterns)
		case FileTypeGitLab:
			fileResults, err = parseGitLabForCheck(filePath, checkSyntaxOnly, ignorePatterns)
		default:
			fileResults, err = parseDockerfileForCheck(filePath, checkSyntaxOnly, ignorePatterns)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filePath, err)
			continue
		}
		baseIdx := len(allResults)
		allResults = append(allResults, fileResults...)
		for i, r := range fileResults {
			if r.Status == "pending" {
				needsCheck = append(needsCheck, baseIdx+i)
			}
		}
	}

	// Phase 2: verify digests in parallel
	if len(needsCheck) > 0 {
		sem := make(chan struct{}, runtime.NumCPU()*2)
		var wg sync.WaitGroup
		for _, idx := range needsCheck {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// Each goroutine exclusively owns allResults[i], no mutex needed
				verifyCheckResult(ctx, res, &allResults[i], checkVerifyTag)
			}(idx)
		}
		wg.Wait()
	}

	// Phase 3: output
	hasFail := false
	switch checkFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(allResults)
	default:
		for _, r := range allResults {
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
	for _, r := range allResults {
		if r.Status == "fail" {
			hasFail = true
			break
		}
	}

	if hasFail {
		os.Exit(checkExitCode)
	}
	return nil
}

// verifyCheckResult resolves a pending CheckResult against the registry.
// r.Message holds the pinned digest on entry and the final message on return.
// When verifyTag is true, the tag must also currently resolve to the pinned digest.
func verifyCheckResult(ctx context.Context, res resolver.DigestResolver, r *CheckResult, verifyTag bool) {
	digest := r.Message
	exists, err := res.Exists(ctx, r.Image+"@"+digest)
	if err != nil {
		r.Status = "warn"
		r.Message = fmt.Sprintf("registry check failed: %v", err)
		return
	}
	if !exists {
		r.Status = "fail"
		r.Message = "digest not found in registry"
		return
	}
	r.Status = "ok"
	r.Message = ""
	if !verifyTag {
		return
	}
	if !hasExplicitTag(r.Image) {
		// e.g. "node@sha256:..." has no tag to compare against
		r.Message = "no tag to verify"
		return
	}
	current, err := res.Resolve(ctx, r.Image)
	if err != nil {
		r.Status = "warn"
		r.Message = fmt.Sprintf("tag resolve failed: %v", err)
		return
	}
	if current != digest {
		r.Status = "fail"
		r.Message = fmt.Sprintf("tag points to a different digest: %s", current)
	}
}

// hasExplicitTag reports whether imageRef (without digest) has an explicit tag.
// A colon before the last "/" belongs to a registry port, not a tag.
func hasExplicitTag(imageRef string) bool {
	lastSlash := strings.LastIndex(imageRef, "/")
	return strings.Contains(imageRef[lastSlash+1:], ":")
}

// parseDockerfileForCheck parses a Dockerfile and returns CheckResults.
// Results that need registry verification have Status="pending" and Message=digest.
func parseDockerfileForCheck(filePath string, syntaxOnly bool, ignoreImages []string) ([]CheckResult, error) {
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
		if IsIgnored(inst.ImageRef, ignoreImages) {
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
		// Needs registry check: store digest in Message temporarily
		results = append(results, CheckResult{
			File: filePath, Line: inst.StartLine, Image: inst.ImageRef,
			Status: "pending", Message: inst.Digest, Original: inst.Original,
		})
	}
	return results, nil
}

// parseComposeForCheck parses a compose file and returns CheckResults.
// Results that need registry verification have Status="pending" and Message=digest.
func parseComposeForCheck(filePath string, syntaxOnly bool, ignoreImages []string) ([]CheckResult, error) {
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
		if IsIgnored(ref.ImageRef, ignoreImages) {
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
		results = append(results, CheckResult{
			File: filePath, Line: ref.Line, Image: ref.ImageRef,
			Status: "pending", Message: ref.Digest, Original: "image: " + ref.RawRef,
		})
	}
	return results, nil
}

func parseActionsForCheck(filePath string, syntaxOnly bool, ignoreImages []string) ([]CheckResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}
	refs, err := actions.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	var results []CheckResult
	for _, ref := range refs {
		// Build Original with YAML key prefix for consistent output
		// Location ends with the key name (e.g., "jobs.test.container.image" → "image")
		original := actionsOriginal(ref)
		if ref.Skip {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "skip", Message: ref.SkipReason, Original: original,
			})
			continue
		}
		if IsIgnored(ref.ImageRef, ignoreImages) {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "skip", Message: "ignored", Original: original,
			})
			continue
		}
		if ref.Digest == "" {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "fail", Message: "missing digest", Original: original,
			})
			continue
		}
		if syntaxOnly {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "ok", Message: "", Original: original,
			})
			continue
		}
		results = append(results, CheckResult{
			File: filePath, Line: ref.Line, Image: ref.ImageRef,
			Status: "pending", Message: ref.Digest, Original: original,
		})
	}
	return results, nil
}

func parseGitLabForCheck(filePath string, syntaxOnly bool, ignoreImages []string) ([]CheckResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}
	refs, err := gitlab.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filePath, err)
	}
	var results []CheckResult
	for _, ref := range refs {
		original := gitlabOriginal(ref)
		if ref.Skip {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "skip", Message: ref.SkipReason, Original: original,
			})
			continue
		}
		if IsIgnored(ref.ImageRef, ignoreImages) {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "skip", Message: "ignored", Original: original,
			})
			continue
		}
		if ref.Digest == "" {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "fail", Message: "missing digest", Original: original,
			})
			continue
		}
		if syntaxOnly {
			results = append(results, CheckResult{
				File: filePath, Line: ref.Line, Image: ref.ImageRef,
				Status: "ok", Message: "", Original: original,
			})
			continue
		}
		results = append(results, CheckResult{
			File: filePath, Line: ref.Line, Image: ref.ImageRef,
			Status: "pending", Message: ref.Digest, Original: original,
		})
	}
	return results, nil
}

// gitlabOriginal renders the source line for check output. A scalar service
// entry has no key, so it is shown with the sequence dash.
func gitlabOriginal(ref gitlab.GitLabImageRef) string {
	if ref.Key == "" {
		return "- " + ref.RawRef
	}
	return ref.Key + ": " + ref.RawRef
}

// actionsOriginal returns a human-readable "key: value" string for check output,
// matching the compose convention (e.g., "image: node:24", "uses: docker://...").
func actionsOriginal(ref actions.ActionsImageRef) string {
	loc := ref.Location
	if idx := strings.LastIndex(loc, "."); idx >= 0 {
		key := loc[idx+1:]
		return key + ": " + ref.RawRef
	}
	return ref.RawRef
}
