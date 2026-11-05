# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.6"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.6/unibox_0.7-pre.6_darwin_arm64.tar.gz"
      sha256 "74b6144d0c068ef7b25adc21de6afc4327674615fa5b2caf7c9bad0a41096c4c"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.6/unibox_0.7-pre.6_darwin_amd64.tar.gz"
      sha256 "4069f062d03dbea0f34e7677631fe4926013a134ba0fa1dc4529ff204c503d2f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.6/unibox_0.7-pre.6_linux_arm64.tar.gz"
      sha256 "995f7b82fe107b763aff13f99d1ac0e97c0c7873fbc46778f55366a7adb2563f"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.6/unibox_0.7-pre.6_linux_amd64.tar.gz"
      sha256 "517fe12cc0abaae1d455699b54a7755a4775b822f228ac9daf5811fa39576240"
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
