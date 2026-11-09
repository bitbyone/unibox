# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.9"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.9/unibox_0.9_darwin_arm64.tar.gz"
      sha256 "b1061be6a77af23e26d990ce43b01b72ef7caf5dce655485e5c466db45f0bff1"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.9/unibox_0.9_darwin_amd64.tar.gz"
      sha256 "783ae6cf66771429f4fc5c0d8199b664c1c9760da2b158f741f26b3a4df28abb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.9/unibox_0.9_linux_arm64.tar.gz"
      sha256 "393857cdb716fe24ff62c356c5b2bf169f3c642dd932ac28decd7ec06dd199e4"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.9/unibox_0.9_linux_amd64.tar.gz"
      sha256 "4f306736678dbc5f09e2fb8b06b8090d6ce4a1f048960c9a5d79dd7a55739893"
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
