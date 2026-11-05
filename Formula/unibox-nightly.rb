# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7-pre.5"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.5/unibox_0.7-pre.5_darwin_arm64.tar.gz"
      sha256 "2fd56f080129fa0e812085a30d4450da9238959608405a6c7ee27a1d3decd90a"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.5/unibox_0.7-pre.5_darwin_amd64.tar.gz"
      sha256 "6efefd278e27fa81f30ced0a9ddee1238962ac48f4c738b13fb5b87d59f0d0be"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.5/unibox_0.7-pre.5_linux_arm64.tar.gz"
      sha256 "a176c490bedc1b02e048cb123699fee301cb6e80dd712721996cb3bf8a00a050"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7-pre.5/unibox_0.7-pre.5_linux_amd64.tar.gz"
      sha256 "07aeb146452acbe5f885b6b18603902d7af45278b29ecd0cff2459f7f46a2d95"
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
