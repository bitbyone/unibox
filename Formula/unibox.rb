# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.5"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.5/unibox_0.5_darwin_arm64.tar.gz"
      sha256 "d4fb7b26f810be2e068d7b202ab2727641863715e17988da7e044a26064ba093"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.5/unibox_0.5_darwin_amd64.tar.gz"
      sha256 "6631a1a1e5436944cfe5aaef8dece8ae47a0af68f4d5c4aa17257a97dbdfc317"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.5/unibox_0.5_linux_arm64.tar.gz"
      sha256 "c1a20bc47a7afc78223458441efda5b4cf30beb1d27929969d4c3ea58f6aa9ec"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.5/unibox_0.5_linux_amd64.tar.gz"
      sha256 "546e51b23a978cff0f0126cf8ea384b526b67343e3aee453e4a49e15dab29be1"
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
