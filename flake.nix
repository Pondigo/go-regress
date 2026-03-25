{
  description = "go-regress: visual regression testing for Go with OpenCV";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            opencv
            pkg-config
          ];

          env = {
            CGO_ENABLED = "1";
          };

          shellHook = ''
            echo "go-regress dev shell loaded"
            echo "  go $(go version | cut -d' ' -f3)"
            echo "  opencv $(pkg-config --modversion opencv4 2>/dev/null || echo 'not found')"
          '';
        };
      }
    );
}
