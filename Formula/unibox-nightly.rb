# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.10-pre.3"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.3/unibox_0.10-pre.3_darwin_arm64.tar.gz"
      sha256 "c05fad540d44764e5fd25fed195ffe95c768ce60e3320ef4c412d6fccc17878e"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.3/unibox_0.10-pre.3_darwin_amd64.tar.gz"
      sha256 "9c2ac2843356d87379cc573a0418af193b717e84214dfd85ff12c6847b3ecb98"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.3/unibox_0.10-pre.3_linux_arm64.tar.gz"
      sha256 "0b2cd744dd90ea7421c11550c4642af54f3c468796a83dee22acf12ea68a1cf8"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.3/unibox_0.10-pre.3_linux_amd64.tar.gz"
      sha256 "c1e5186fb9e3477163b3fe7ac68ba733d2ac54c67ba9378ceeb6b14313fafbdc"
    end
  end

  head do
    url "https://github.com/bitbyone/unibox.git", branch: "main"
    depends_on "go" => :build
  end

  def install
    if build.head?
      system "go", "build", *std_go_args(ldflags: "-X github.com/tobola/unibox/internal/version.Version=HEAD"), "./cmd/unibox"
    else
      bin.install "unibox"
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/unibox --version")
  end
end
