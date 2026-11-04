# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.3"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.3/unibox_0.7-pre.3_darwin_arm64.tar.gz"
      sha256 "ce2009e57776adc108a757a1506416f1be6c192ff8ccd06002d5935981f40e80"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.3/unibox_0.7-pre.3_darwin_amd64.tar.gz"
      sha256 "6856d1872d6eac73bcd9571de67f34d6ec72735320baeadb591063d43024616b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.3/unibox_0.7-pre.3_linux_arm64.tar.gz"
      sha256 "81f01b95f9f4ea08c3af491703e470581348e97e7ac2f7de354e3691373d21c1"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.3/unibox_0.7-pre.3_linux_amd64.tar.gz"
      sha256 "77a1d756377032c29cb435324e343b8d73f64a47151a4aa143f8a697bc4a1aaf"
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
