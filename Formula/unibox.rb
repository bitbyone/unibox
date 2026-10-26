# The release workflow (.github/workflows/release.yml) rewrites the tag and
# the revision below when a vX.Y tag is pushed; Homebrew refuses to build if
# the tag no longer points at that revision. Edit the rest by hand.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  url "https://github.com/bitbyone/unibox.git",
      tag:      "v0.2",
      revision: "06af3564d989d743d618d10f95b22721f04c2e3a"
  license "MIT"
  head "https://github.com/bitbyone/unibox.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-X github.com/tobola/unibox/internal/version.Version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/unibox"
  end

  test do
    assert_equal version.to_s, shell_output("#{bin}/unibox --version").strip
  end
end
