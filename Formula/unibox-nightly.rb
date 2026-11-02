# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.1"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.1/unibox_0.7-pre.1_darwin_arm64.tar.gz"
      sha256 "d7c31e89474aa3b9d34b3db6ce309c793c0cf7273215a6df4986e857692dd53b"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.1/unibox_0.7-pre.1_darwin_amd64.tar.gz"
      sha256 "6b03a9b03a44b1f9baaf48f56e82b1476d20264fa2d5277418babde98742887c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.1/unibox_0.7-pre.1_linux_arm64.tar.gz"
      sha256 "3bba71ac171ecb8c9d5ac56fe2eb0ac763a253ddafcb109e57715c0da650a74c"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.1/unibox_0.7-pre.1_linux_amd64.tar.gz"
      sha256 "331d3b9cf5b9d7408345d4fe757cd6bbb3fdb046def8e03710f14833a2c16bef"
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
