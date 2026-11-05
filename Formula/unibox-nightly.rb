# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.4"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.4/unibox_0.7-pre.4_darwin_arm64.tar.gz"
      sha256 "022092fe7510df3a2c3597178af6b4dedbc96ee0cced28aba3141756af46397a"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.4/unibox_0.7-pre.4_darwin_amd64.tar.gz"
      sha256 "96460d30c58c3851f7f095903f9d268410778c453f50b036aa564460b219d825"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.4/unibox_0.7-pre.4_linux_arm64.tar.gz"
      sha256 "af364d6f6779559effde8ed86e655a8aee24dc787384eacceb52b9ae6f9787b4"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.4/unibox_0.7-pre.4_linux_amd64.tar.gz"
      sha256 "5a10e4b4c9013eb01a04509ddd275bf94fbd91ba6c46d4a4fef86af5badd4451"
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
