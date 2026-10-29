# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.2"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.2/unibox_0.2_darwin_arm64.tar.gz"
      sha256 "73de17c8576eacc490f8b2769fd3d05ef2384832c09872dabb9f5ca2c31b7e96"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.2/unibox_0.2_darwin_amd64.tar.gz"
      sha256 "baa1890f1dd028d33156cc5c817e08380a71481f9a437168b229b15184ef33db"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.2/unibox_0.2_linux_arm64.tar.gz"
      sha256 "d9b5b10bad5f6624353ed1ae9a0814d4774801ac05875428b3aeb33d462b6531"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.2/unibox_0.2_linux_amd64.tar.gz"
      sha256 "4417a50aabab57e9e0388b4ac5c7706b0870493f3627f067984b6befe784e7e3"
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
