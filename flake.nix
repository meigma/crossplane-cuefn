{
  description = "cuefn — the crossplane-cuefn CLI: render CUE modules, generate XRDs, and package Crossplane Configurations";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        # Kept in sync with releases by release-please (see release-please-config.json).
        version = "0.1.4"; # x-release-please-version
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "cuefn";
          inherit version;

          # Build from the flake's own source tree; a consumer pins the version by
          # pinning the git ref (e.g. github:meigma/crossplane-cuefn/v0.1.2).
          src = ./.;

          # Regenerate after any go.sum change: set to pkgs.lib.fakeHash, run
          # `nix build`, and copy the sha256 from the mismatch error.
          vendorHash = "sha256-t5BISl+SDgRk8jbWXVRx6TrCSIjEoVob72O1zJUL8i8=";

          subPackages = [ "cmd/cuefn" ];
          env.CGO_ENABLED = 0;

          # The heavy suites need Docker/network and self-skip; unit tests run in CI.
          # A source build should not run the test suite.
          doCheck = false;

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
            "-X main.commit=${self.rev or "unknown"}"
            "-X main.date=${self.lastModifiedDate}"
          ];

          meta = {
            description = "Crossplane v2 composition function CLI that renders Kubernetes resources from CUE modules";
            homepage = "https://github.com/meigma/crossplane-cuefn";
            license = with pkgs.lib.licenses; [ asl20 mit ];
            mainProgram = "cuefn";
          };
        };
      }
    );
}
