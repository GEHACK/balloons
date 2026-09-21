{
  description = "Balloons — DOMjudge balloon dispatcher for GEHACK (Rust rewrite)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    { nixpkgs, flake-utils, rust-overlay, ... }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ (import rust-overlay) ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            # wasm32-unknown-unknown is what `dx` builds the Dioxus client into.
            (pkgs.rust-bin.stable.latest.default.override {
              extensions = [ "rust-src" "rust-analyzer" "clippy" "rustfmt" ];
              targets = [ "wasm32-unknown-unknown" ];
            })

            # Keep the `dioxus` crate version in step with `dx --version`.
            pkgs.dioxus-cli

            pkgs.just
          ];
        };
      }
    );
}
