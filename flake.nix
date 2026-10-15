{
  description = "SD project task runner";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        version = "0.1.0";
        sd = pkgs.buildGoModule {
          pname = "sd";
          inherit version;
          src = ./.;
          vendorHash = "sha256-dmLyUY2cpCXrHCF+fCn++19aRwbS8bOqK3LGz4F8IGY=";

          subPackages = [ "cmd/sd" ];

          ldflags = [
            "-s"
            "-w"
            "-X github.com/self-deprecated/sd/internal/cli.Version=${version}"
          ];

          nativeBuildInputs = [ pkgs.installShellFiles ];

          postInstall = ''
            mkdir -p completions
            $out/bin/sd completion bash > completions/sd.bash
            $out/bin/sd completion zsh > completions/_sd
            $out/bin/sd completion fish > completions/sd.fish

            installShellCompletion \
              --bash completions/sd.bash \
              --zsh completions/_sd \
              --fish completions/sd.fish
          '';

          meta = {
            description = "Self Deprecated project task runner";
            mainProgram = "sd";
          };
        };
      in
      {
        packages.sd = sd;
        packages.default = sd;

        apps.sd = {
          type = "app";
          program = "${sd}/bin/sd";
          meta.description = "Run the SD project task runner";
        };
        apps.default = self.apps.${system}.sd;

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls pkgs.gotools pkgs.golangci-lint pkgs.nixpkgs-fmt ];
        };
      });
}
