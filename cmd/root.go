package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "dockerfile-pin",
	Short: "Pin Dockerfile and docker-compose images to digests",
	Long: `Pin Docker image references to their sha256 digests to prevent supply chain attacks.

Supported file types:
  - Dockerfile          FROM image:tag → FROM image:tag@sha256:...
  - docker-compose.yml  image: node:20 → image: node:20@sha256:...
  - GitHub Actions      container/services/docker:// steps
  - action.yml          runs.image: 'docker://...'

File detection:
  When neither -f nor --glob is given, auto-detects files via git ls-files
  matching: Dockerfile, Dockerfile.*, docker-compose*.yml, compose.yaml,
  action.yml, .github/workflows/*.yml, etc.
  Outside a git repo, falls back to the same glob (skipping node_modules/, vendor/).

Configuration:
  Create .dockerfile-pin.yaml in project root to set persistent ignore rules:
    ignore-images:
      - "ghcr.io/myorg/*"           # glob patterns
      - "!ghcr.io/myorg/public-*"   # negation (! prefix) re-enables checking
  CLI --ignore-images flags are appended after config (last match wins).

Authentication:
  Uses ~/.docker/config.json for registry auth (Docker Hub, GHCR, GCR, ECR, etc.).
  Run "docker login" or configure a credential helper before accessing private registries.

Typical workflow:
  dockerfile-pin run              # preview changes (dry-run)
  dockerfile-pin run --write      # apply changes
  dockerfile-pin check            # validate in CI`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
