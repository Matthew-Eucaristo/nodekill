# Homebrew formula for Matthew-Eucaristo/homebrew-tap (Formula/nodekill.rb).
# Builds from the tagged source tarball. Bump `url` and `sha256` per release;
# see RELEASING.md.
class Nodekill < Formula
  desc "Reclaim disk space from node_modules, JS build caches, npm/pnpm/yarn/bun stores and old Node versions"
  homepage "https://github.com/Matthew-Eucaristo/nodekill"
  url "https://github.com/Matthew-Eucaristo/nodekill/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "REPLACE_WITH_SHA256_OF_THE_TAG_TARBALL"
  license "MIT"
  head "https://github.com/Matthew-Eucaristo/nodekill.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}")
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/nodekill --version")
    assert_match "\"schema_version\": 1", shell_output("#{bin}/nodekill capabilities --json")

    (testpath/"dev/app/package.json").write '{"dependencies":{"next":"16"}}'
    (testpath/"dev/app/package-lock.json").write "{}"
    (testpath/"dev/app/node_modules/next/index.js").write "x"
    output = shell_output("#{bin}/nodekill scan --json --no-global #{testpath}/dev")
    assert_match "node_modules", output
    assert_path_exists testpath/"dev/app/node_modules" # scan never changes anything
  end
end
