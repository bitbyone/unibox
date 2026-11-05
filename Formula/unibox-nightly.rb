# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.8"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.8/unibox_0.7-pre.8_darwin_arm64.tar.gz"
      sha256 "270e8312ffd99791712fe1a04489504201e95220af66498bb531f105dccfb8e2"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.8/unibox_0.7-pre.8_darwin_amd64.tar.gz"
      sha256 "7597983ad9084fd6de75c766c875ed534e3eeb449b6ea0b8c54ab91ae2f1ec2f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.8/unibox_0.7-pre.8_linux_arm64.tar.gz"
      sha256 "c610a0765f54aa234a42e0712c5847319d0df59acc72d5bbecaa5d30862f3997"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.8/unibox_0.7-pre.8_linux_amd64.tar.gz"
      sha256 "36f0ec3d0b4c753f834e8a702607467c1132c0aa79ac723a00ed24d36b841618"
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
