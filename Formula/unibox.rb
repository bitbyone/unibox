# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.6"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.6/unibox_0.6_darwin_arm64.tar.gz"
      sha256 "aff77cf38291424592a1e1630753251190af6ff1f1f00643df3090bc0dff6cf8"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.6/unibox_0.6_darwin_amd64.tar.gz"
      sha256 "6dfc8a1542749b702d68f49ab1edb24ca12284017a67f3337f98b7992f48e43c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.6/unibox_0.6_linux_arm64.tar.gz"
      sha256 "278ca686e10c8917c37516e52cfbf80c3c53087d49bde84e1cfeaf6cc0c5cf80"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.6/unibox_0.6_linux_amd64.tar.gz"
      sha256 "bcd73efd9f5c4e96526839515cf9ec690118f00f212b39f391ff54459435bfa6"
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
