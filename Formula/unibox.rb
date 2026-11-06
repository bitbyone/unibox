# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.8"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.8/unibox_0.8_darwin_arm64.tar.gz"
      sha256 "eb27882b0ec2c00e6a4c3edd7c10035d77892b7140a8e3e5bbf74203c6f375cb"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.8/unibox_0.8_darwin_amd64.tar.gz"
      sha256 "1085d40976ca9f11f6f76691ba093a7f08e2a7f6f654366b5d381835d6267694"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.8/unibox_0.8_linux_arm64.tar.gz"
      sha256 "ffe5cc13e7e69f9293712534b3d0db952715c0df3e72f048f770f0f91171f721"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.8/unibox_0.8_linux_amd64.tar.gz"
      sha256 "87f8bba224f0a155ed28f8ecdc6874de597f5aced6b56360b18cd1070b865ea4"
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
