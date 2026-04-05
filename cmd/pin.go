package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/azu/dockerfile-pin/internal/actions"
	"github.com/azu/dockerfile-pin/internal/compose"
	"github.com/azu/dockerfile-pin/internal/dockerfile"
	"github.com/azu/dockerfile-pin/internal/resolver"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Pin FROM images to their digests",
	Long: `Resolve image tags to sha256 digests and add @sha256:<digest> to each reference.
By default, prints the rewritten file to stdout without modifying it (dry-run).
Use --write to apply changes in place.

Supports Dockerfiles, docker-compose.yml/compose.yaml, GitHub Actions workflows,
and action.yml files. File type is detected from filename.

Skipped automatically:
  - "FROM scratch" (no registry image)
  - Multi-stage references ("FROM builder")
  - ARG-only base images with no default value
  - Compose services with a "build:" directive
  - Non-docker "uses:" in GitHub Actions (e.g., actions/checkout@v4)
  - Already-pinned images (use --update to re-resolve)

Digests are resolved via HEAD requests and do not count against Docker Hub pull limits.`,
	Example: `  # Preview changes for auto-detected files
  dockerfile-pin run

  # Apply changes
  dockerfile-pin run --write

  # Pin a specific file
  dockerfile-pin run -f path/to/Dockerfile --write

  # Pin all Dockerfiles and compose files
  dockerfile-pin run --glob '**/{Dockerfile,docker-compose.yml}' --write

  # Re-resolve existing digests (tag unchanged, digest updated)
  dockerfile-pin run --write --update

  # Ignore private registry images
  dockerfile-pin run --ignore-images "*.dkr.ecr.*.amazonaws.com/**"`,
	Args: cobra.NoArgs,
	RunE: runRun,
}

var (
	runFilePath string
	runGlob     string
	runWrite    bool
	runUpdate   bool
	runIgnore   []string
)

func init() {
	runCmd.Flags().StringVarP(&runFilePath, "file", "f", "", "Dockerfile path (default: auto-detect)")
	runCmd.Flags().StringVar(&runGlob, "glob", "", "Glob pattern to find Dockerfiles")
	runCmd.Flags().BoolVar(&runWrite, "write", false, "Write changes to files (default is dry-run)")
	runCmd.Flags().BoolVar(&runUpdate, "update", false, "Update existing digests")
	runCmd.Flags().StringArrayVar(&runIgnore, "ignore-images", nil, "Images to ignore (glob patterns, repeatable)")
	rootCmd.AddCommand(runCmd)
}

// parsedFile holds the parsed result of a single file.
type parsedFile struct {
	path        string
	fileType    FileType
	dockerInsts []dockerfile.FromInstruction
	composeRefs []compose.ComposeImageRef
	actionsRefs []actions.ActionsImageRef
	content     []byte
	imageRefs   []string // unique image refs that need resolving
}

func runRun(cmd *cobra.Command, args []string) error {
	files, err := FindFiles(runFilePath, runGlob)
	if err != nil {
		return err
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	ignorePatterns := MergeIgnorePatterns(cfg.IgnoreImages, runIgnore)
	if err := ValidatePatterns(ignorePatterns); err != nil {
		return err
	}

	dryRun := !runWrite
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Printf("found %d file(s)\n", len(files))

	// Phase 1: Parse all files and collect unique image refs
	var parsed []parsedFile
	uniqueRefs := make(map[string]bool)

	for _, filePath := range files {
		pf, err := parseFile(filePath, runUpdate, ignorePatterns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", filePath, err)
			continue
		}
		parsed = append(parsed, pf)
		for _, ref := range pf.imageRefs {
			uniqueRefs[ref] = true
		}
	}

	// Phase 2: Resolve all unique digests in parallel
	refs := make([]string, 0, len(uniqueRefs))
	for ref := range uniqueRefs {
		refs = append(refs, ref)
	}
	fmt.Printf("resolving %d unique image(s)...\n", len(refs))

	res := &resolver.CraneResolver{}
	digestMap := resolveParallel(ctx, res, refs)

	// Phase 3: Apply digests and output results
	for _, pf := range parsed {
		switch pf.fileType {
		case FileTypeCompose:
			applyCompose(pf, digestMap, dryRun, runUpdate)
		case FileTypeActions:
			applyActions(pf, digestMap, dryRun, runUpdate)
		default:
			applyDockerfile(pf, digestMap, dryRun, runUpdate)
		}
	}
	return nil
}

func parseFile(filePath string, update bool, ignorePatterns []string) (parsedFile, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return parsedFile{}, fmt.Errorf("reading %s: %w", filePath, err)
	}

	pf := parsedFile{
		path:     filePath,
		fileType: DetectFileType(filePath),
		content:  content,
	}

	switch pf.fileType {
	case FileTypeCompose:
		refs, err := compose.Parse(content)
		if err != nil {
			return parsedFile{}, fmt.Errorf("parsing %s: %w", filePath, err)
		}
		pf.composeRefs = refs
		for _, ref := range refs {
			if ref.Skip || (ref.Digest != "" && !update) || IsIgnored(ref.ImageRef, ignorePatterns) {
				continue
			}
			pf.imageRefs = append(pf.imageRefs, ref.ImageRef)
		}
	case FileTypeActions:
		refs, err := actions.Parse(content)
		if err != nil {
			return parsedFile{}, fmt.Errorf("parsing %s: %w", filePath, err)
		}
		pf.actionsRefs = refs
		for _, ref := range refs {
			if ref.Skip || (ref.Digest != "" && !update) || IsIgnored(ref.ImageRef, ignorePatterns) {
				continue
			}
			pf.imageRefs = append(pf.imageRefs, ref.ImageRef)
		}
	default:
		insts, err := dockerfile.Parse(strings.NewReader(string(content)))
		if err != nil {
			return parsedFile{}, fmt.Errorf("parsing %s: %w", filePath, err)
		}
		pf.dockerInsts = insts
		for _, inst := range insts {
			if inst.Skip || (inst.Digest != "" && !update) || IsIgnored(inst.ImageRef, ignorePatterns) {
				continue
			}
			pf.imageRefs = append(pf.imageRefs, inst.ImageRef)
		}
	}
	return pf, nil
}

// resolveParallel resolves digests for all image refs concurrently.
func resolveParallel(ctx context.Context, res resolver.DigestResolver, refs []string) map[string]string {
	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU()*2)

	for _, ref := range refs {
		wg.Add(1)
		go func(imageRef string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			digest, err := res.Resolve(ctx, imageRef)
			mu.Lock()
			if err != nil {
				fmt.Fprintf(os.Stderr, "WARN  %s  failed to resolve: %v\n", imageRef, err)
			} else {
				results[imageRef] = digest
				fmt.Printf("  resolved %s → %s\n", imageRef, digest[:min(19, len(digest))])
			}
			mu.Unlock()
		}(ref)
	}
	wg.Wait()
	return results
}

// writeFilePreservingPerms writes data to path using the file's existing permissions,
// falling back to 0644 if the permissions cannot be read.
func writeFilePreservingPerms(path string, data []byte) error {
	perm := os.FileMode(0644)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	return os.WriteFile(path, data, perm)
}

func applyDockerfile(pf parsedFile, digestMap map[string]string, dryRun bool, update bool) {
	digests := make(map[int]string)
	for i, inst := range pf.dockerInsts {
		if inst.Skip || (inst.Digest != "" && !update) {
			continue
		}
		if d, ok := digestMap[inst.ImageRef]; ok {
			digests[i] = d
		}
	}
	if len(digests) == 0 {
		return
	}
	result := dockerfile.RewriteFile(string(pf.content), pf.dockerInsts, digests)
	if dryRun {
		fmt.Printf("--- %s\n", pf.path)
		fmt.Print(result)
		return
	}
	if err := writeFilePreservingPerms(pf.path, []byte(result)); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", pf.path, err)
		return
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), pf.path)
}

func applyActions(pf parsedFile, digestMap map[string]string, dryRun bool, update bool) {
	digests := make(map[int]string)
	for i, ref := range pf.actionsRefs {
		if ref.Skip || (ref.Digest != "" && !update) {
			continue
		}
		if d, ok := digestMap[ref.ImageRef]; ok {
			digests[i] = d
		}
	}
	if len(digests) == 0 {
		return
	}
	result := actions.RewriteFile(string(pf.content), pf.actionsRefs, digests)
	if dryRun {
		fmt.Printf("--- %s\n", pf.path)
		fmt.Print(result)
		return
	}
	if err := writeFilePreservingPerms(pf.path, []byte(result)); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", pf.path, err)
		return
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), pf.path)
}

func applyCompose(pf parsedFile, digestMap map[string]string, dryRun bool, update bool) {
	digests := make(map[int]string)
	for i, ref := range pf.composeRefs {
		if ref.Skip || (ref.Digest != "" && !update) {
			continue
		}
		if d, ok := digestMap[ref.ImageRef]; ok {
			digests[i] = d
		}
	}
	if len(digests) == 0 {
		return
	}
	result := compose.RewriteFile(string(pf.content), pf.composeRefs, digests)
	if dryRun {
		fmt.Printf("--- %s\n", pf.path)
		fmt.Print(result)
		return
	}
	if err := writeFilePreservingPerms(pf.path, []byte(result)); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", pf.path, err)
		return
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), pf.path)
}
