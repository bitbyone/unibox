# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.11"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.11/unibox_0.11_darwin_arm64.tar.gz"
      sha256 "88484da022570273011b44d275977eab8f4d544b49544f485181cb29fe800432"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.11/unibox_0.11_darwin_amd64.tar.gz"
      sha256 "ee7d1248ffaeec95b934f3dbfc965afb149df361047584e52ac22b85b0cff014"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.11/unibox_0.11_linux_arm64.tar.gz"
      sha256 "5aedf0c5a860b0144900d739eed88acca3164d171e6db490e8b016f6c814e276"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.11/unibox_0.11_linux_amd64.tar.gz"
      sha256 "a2be39474670a534dffab5b7a5a682895e8e46503eb7d555ed936312fdcd4e76"
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
