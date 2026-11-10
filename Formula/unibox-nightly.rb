# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.10-pre.1"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.1/unibox_0.10-pre.1_darwin_arm64.tar.gz"
      sha256 "ca493df719ee70335c0195484a31c5ca4396c2281ebec12baa13af100cd5e753"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.1/unibox_0.10-pre.1_darwin_amd64.tar.gz"
      sha256 "c5ea3c6ff165cb3111bb17712182b373bf50b57d2b547f3559aa422be82044f0"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.1/unibox_0.10-pre.1_linux_arm64.tar.gz"
      sha256 "82a231c29bba49308de841a275afc3b65b6bea1ab21f74dca128a0d1e96fd33d"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.1/unibox_0.10-pre.1_linux_amd64.tar.gz"
      sha256 "52c1a314ba61579ae70bbc0bbc47644c090f2d1895da732343086c5a80861f8f"
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
