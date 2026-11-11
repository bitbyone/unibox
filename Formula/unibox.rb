# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.10"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10/unibox_0.10_darwin_arm64.tar.gz"
      sha256 "db6ab58fd51022d66e85e2115b5b0706748a7de538ef3b3e48d6154fb6fade91"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10/unibox_0.10_darwin_amd64.tar.gz"
      sha256 "ebe66cd726c4de07ed7aed6ef9ee3f4eabfc5ec1170ff277e5830de1b3444cf4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10/unibox_0.10_linux_arm64.tar.gz"
      sha256 "5f5b3a08900722285d577fcf8918afee5005a8489771cb7b646ec06d37eca414"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.10/unibox_0.10_linux_amd64.tar.gz"
      sha256 "d990b77bf4328a1543c7ad72527c0104b25d271e988056fdea1df20c1bf2540f"
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
