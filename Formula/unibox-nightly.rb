# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.2"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.2/unibox_0.7-pre.2_darwin_arm64.tar.gz"
      sha256 "69ee4f077d4f8273b2532cb0f6465db305fadea102a7f0e6447773bc89c6ecb1"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.2/unibox_0.7-pre.2_darwin_amd64.tar.gz"
      sha256 "49a531ecb1a54bfe3c62bdd13180c0a069696f735e7e4e5865abe3c2c7e97214"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.2/unibox_0.7-pre.2_linux_arm64.tar.gz"
      sha256 "e6802dba143f07db017fef6254a1881bfc53b64ab17a83d60ded1f069a9038d6"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.2/unibox_0.7-pre.2_linux_amd64.tar.gz"
      sha256 "76908a30e5f056bf291c4ff759d3ab8ce403a6dee2bb80d9ec591e6854b002e6"
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
