# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.4"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.4/unibox_0.4_darwin_arm64.tar.gz"
      sha256 "5d832aad7eb1be7eaf28adccd4c98edc6f7aeb45728d9d89a40bc1acc1fdff2e"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.4/unibox_0.4_darwin_amd64.tar.gz"
      sha256 "be26e72f45d7e30c9ae8e47241fb289205f5370f35ba717e60f224df0ef4f4a4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.4/unibox_0.4_linux_arm64.tar.gz"
      sha256 "88df7e362f158a3eb02c3b4ccc9ee600cd566e7411b6027c5640672026ce4980"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.4/unibox_0.4_linux_amd64.tar.gz"
      sha256 "6791fa157cf17c7543c83701d2541ba4c834ad811546b08a327a6ef000277eaf"
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
