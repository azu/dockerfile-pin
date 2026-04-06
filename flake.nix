{
  description = "A CLI tool for digest pinning — adds @sha256:<digest> to Dockerfile, docker-compose.yml, and GitHub Actions to prevent supply chain attacks";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    let
      version = self.shortRev or "dev";
    in
    {
      packages = forAllSystems (pkgs: {
        dockerfile-pin = pkgs.buildGoModule {
          pname = "dockerfile-pin";
          inherit version;
          src = ./.;
          vendorHash = "sha256-CgMFIYoM+nWiZ5NXtTlXHhrjzVYxoVg0YVpQq3LLrjI=";

          # Tests require network access (container registry resolution) and git
          doCheck = false;

          ldflags = [
            "-s"
            "-w"
          ];

          meta = {
            description = "A CLI tool for digest pinning — adds @sha256:<digest> to Dockerfile, docker-compose.yml, and GitHub Actions to prevent supply chain attacks";
            homepage = "https://github.com/azu/dockerfile-pin";
            license = pkgs.lib.licenses.mit;
            mainProgram = "dockerfile-pin";
          };
        };
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.dockerfile-pin;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShellNoCC {
          packages = with pkgs; [
            go
            gopls
          ];
        };
      });
    };
}
