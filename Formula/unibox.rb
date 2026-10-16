# The release workflow (.github/workflows/release.yml) rewrites the tag and
# the revision below when a vX.Y tag is pushed; Homebrew refuses to build if
# the tag no longer points at that revision. Edit the rest by hand.
class Unibox < Formula
  desc "Run coding agents in sandboxes defined by profiles in a git repository"
  homepage "https://bitbyone.github.io/unibox/"
  url "https://github.com/bitbyone/unibox.git",
      tag:      "v0.0",
      revision: "0000000000000000000000000000000000000000"
  license "MIT"
  head "https://github.com/bitbyone/unibox.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-X github.com/tobola/unibox/internal/version.Version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/unibox"
  end

  test do
    assert_match "unibox version #{version} (profile format", shell_output("#{bin}/unibox --version")
  end
end
