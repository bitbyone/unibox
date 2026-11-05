# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.7"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.7/unibox_0.7-pre.7_darwin_arm64.tar.gz"
      sha256 "b6e4b0c86df7f1ecc95e805f6e2b6de77864d430b1c8649b5fd0b910d8d21b4a"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.7/unibox_0.7-pre.7_darwin_amd64.tar.gz"
      sha256 "82b5f7b8f2c89eff2dee70cb20c58251df63f47e1003984ba709c2901678331b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.7/unibox_0.7-pre.7_linux_arm64.tar.gz"
      sha256 "6acbccd8c830bfda9702e9ff6ddd6f2702e7d103bb7b177518d35be5885310b4"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.7/unibox_0.7-pre.7_linux_amd64.tar.gz"
      sha256 "63070925b464dc70b30949243463cdedcb33728bdcee9b85381ec2df434399e9"
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
