# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.10-pre.2"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.2/unibox_0.10-pre.2_darwin_arm64.tar.gz"
      sha256 "12bea02f92693c90ec2185c83b45013c701b294ab514777386563aaa3ae16d25"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.2/unibox_0.10-pre.2_darwin_amd64.tar.gz"
      sha256 "af2653f1a5b25eeabf858d9c61f62df0ce03d86eaf9c46276f3732b4e6b631df"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.2/unibox_0.10-pre.2_linux_arm64.tar.gz"
      sha256 "8ce40312c6d143cd7965f9a22dea8cb92c926e182f4665cbd89dceace6765645"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10-pre.2/unibox_0.10-pre.2_linux_amd64.tar.gz"
      sha256 "bb6e6efc8858f7c75a9397a47bd0538157ab9b1c86c5ab131abe29c52d37ed28"
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
