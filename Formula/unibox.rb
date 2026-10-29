# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.3"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.3/unibox_0.3_darwin_arm64.tar.gz"
      sha256 "6e0d2d29aa5cacc4437adc7ccc243f0b3beae540437c2d2b7bee613d4d3867b6"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.3/unibox_0.3_darwin_amd64.tar.gz"
      sha256 "a1aa769928b20c97eadcc56a39bdad416bf7def389d64d0e2d7f9e245fd0b9cd"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.3/unibox_0.3_linux_arm64.tar.gz"
      sha256 "40baf868cd8ccb7c2f2dbe6395b26c7fabb40606aef0fd099d2c8e07f0241768"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.3/unibox_0.3_linux_amd64.tar.gz"
      sha256 "293cb33ac8e97af7b76118f2db23e759b6a5ff412dfaa1fd3ac2a6016197bfea"
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
