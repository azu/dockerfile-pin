package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
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
	Args:  cobra.NoArgs,
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

// parsedFile holds the parsed result of a single file.
type parsedFile struct {
	path        string
	fileType    FileType
	dockerInsts []dockerfile.FromInstruction
	composeRefs []compose.ComposeImageRef
	content     []byte
	imageRefs   []string // unique image refs that need resolving
}

func runRun(cmd *cobra.Command, args []string) error {
	files, err := FindFiles(runFilePath, runGlob)
	if err != nil {
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
		pf, err := parseFile(filePath, runUpdate)
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
			applyCompose(pf, digestMap, dryRun)
		default:
			applyDockerfile(pf, digestMap, dryRun)
		}
	}
	return nil
}

func parseFile(filePath string, update bool) (parsedFile, error) {
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
			if ref.Skip || (ref.Digest != "" && !update) {
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
			if inst.Skip || (inst.Digest != "" && !update) {
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
				fmt.Printf("  resolved %s → %s\n", imageRef, digest[:19])
			}
			mu.Unlock()
		}(ref)
	}
	wg.Wait()
	return results
}

func applyDockerfile(pf parsedFile, digestMap map[string]string, dryRun bool) {
	digests := make(map[int]string)
	for i, inst := range pf.dockerInsts {
		if inst.Skip {
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
	if err := os.WriteFile(pf.path, []byte(result), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", pf.path, err)
		return
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), pf.path)
}

func applyCompose(pf parsedFile, digestMap map[string]string, dryRun bool) {
	digests := make(map[int]string)
	for i, ref := range pf.composeRefs {
		if ref.Skip {
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
	if err := os.WriteFile(pf.path, []byte(result), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", pf.path, err)
		return
	}
	fmt.Printf("pinned %d image(s) in %s\n", len(digests), pf.path)
}
