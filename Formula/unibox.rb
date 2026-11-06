# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class Unibox < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.7"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7/unibox_0.7_darwin_arm64.tar.gz"
      sha256 "cc795782ab9af4417cb794538f72626d2c2eecfb9e5544be11441989df639f6e"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7/unibox_0.7_darwin_amd64.tar.gz"
      sha256 "c3fcf90dbc55317c0f66f64c988288f429b7c911a4e36c8e534c92efa514406c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7/unibox_0.7_linux_arm64.tar.gz"
      sha256 "872567912f1fefc57548eca7e1c9867f53bf37a380a3a2f055b470e83e5b181e"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.7/unibox_0.7_linux_amd64.tar.gz"
      sha256 "6fc3e98b4f63ffeadc69c6e8d171ac0035ee09bfe77022689c59b6b9144dce53"
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
