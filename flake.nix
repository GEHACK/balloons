{
  description = "Balloons — DOMjudge balloon dispatcher for GEHACK";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        lib = pkgs.lib;

        # Frontend: install npm deps, run buf generate (for the TS stubs in
        # web/src/gen), then `npm run build` to produce web/dist.
        frontend = pkgs.buildNpmPackage {
          pname = "balloons-web";
          version = "0.1.0";

          src = ./web;

          # Update on first build: replace with the hash nix prints.
          npmDepsHash = "sha256-SSQrRwEbMbvTZUcOQaSU0MN4mGvsv/oRFK9Ocgdt1iQ=";

          nativeBuildInputs = [
            pkgs.buf
            pkgs.protoc-gen-go
            pkgs.protoc-gen-connect-go
          ];

          # buf.gen.yaml + proto/ live at the repo root. Stage a minimal tree
          # next to web/ so `buf generate` can find the proto sources and emit
          # TS into web/src/gen. The protoc-gen-es plugin lives in
          # web/node_modules/.bin and buf picks it up via the relative path in
          # buf.gen.yaml.
          preBuild = ''
            workdir=$(mktemp -d)
            cp -r ${./proto}        $workdir/proto
            cp    ${./buf.yaml}     $workdir/buf.yaml
            cp    ${./buf.gen.yaml} $workdir/buf.gen.yaml
            ln -s "$PWD" "$workdir/web"
            ( cd "$workdir" && buf generate )
          '';

          # buildPhase from buildNpmPackage runs `npm run build` which produces
          # web/dist. We only want to ship dist + the two HTML entry points.
          installPhase = ''
            runHook preInstall
            mkdir -p $out
            cp -r dist $out/dist
            cp index.html scan.html $out/
            runHook postInstall
          '';
        };

        balloons = pkgs.buildGoModule {
          pname = "balloons";
          version = "0.1.0";

          src = ./.;

          # Update on first build: replace with the hash nix prints.
          vendorHash = "sha256-wZpqcC3jlsuUxMKLflu27x5GOaXq1Veaku1wxSrW8N0=";

          subPackages = [ "cmd/server" ];

          nativeBuildInputs = [
            pkgs.buf
            pkgs.protoc-gen-go
            pkgs.protoc-gen-connect-go
            pkgs.makeWrapper
          ];

          # buf.gen.yaml also emits TS into web/src/gen via a node plugin we
          # don't have here; strip that plugin entry before generating Go so
          # `buf generate` doesn't fail looking for protoc-gen-es.
          preBuild = ''
            export HOME=$(mktemp -d)
            ${pkgs.yq-go}/bin/yq -i 'del(.plugins[] | select(.local | test("protoc-gen-es")))' buf.gen.yaml
            buf generate
          '';

          postInstall = ''
            mv $out/bin/server $out/bin/.balloons-unwrapped

            mkdir -p $out/share/balloons/web
            cp -r ${frontend}/dist     $out/share/balloons/web/dist
            cp    ${frontend}/index.html $out/share/balloons/web/
            cp    ${frontend}/scan.html  $out/share/balloons/web/
            cp -r templates $out/share/balloons/templates

            # The server serves ./web and reads ./templates relative to cwd,
            # so wrap to chdir into the share dir. Typst is a runtime dep
            # (the printer subsystem shells out to it).
            makeWrapper $out/bin/.balloons-unwrapped $out/bin/balloons \
              --chdir $out/share/balloons \
              --prefix PATH : ${lib.makeBinPath [ pkgs.typst ]}
          '';

          meta = {
            description = "DOMjudge balloon dispatcher with printer + first-solve support";
            mainProgram = "balloons";
            license = lib.licenses.mit;
            platforms = lib.platforms.unix;
          };
        };
      in
      {
        packages.default = balloons;
        packages.balloons = balloons;
        packages.frontend = frontend;

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            just
            buf
            protoc-gen-go
            protoc-gen-connect-go
            nodejs_22
            typst
          ];
        };
      }
    );
}
