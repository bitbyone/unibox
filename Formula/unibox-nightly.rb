# The release workflow (.github/workflows/release.yml) writes this file
# whole when a vX.Y tag is pushed, from the release's archives and their
# SHA-256 sums; change it there. brew installs the archive's binary, so
# it needs no Go; --HEAD builds main from source.
class UniboxNightly < Formula
  desc "Wrapper over Docker Sandboxes for running coding agents locally"
  homepage "https://bitbyone.github.io/unibox/"
  version "0.12-pre.1"
  license "MIT"

  conflicts_with "unibox", because: "both install the unibox command"

  on_macos do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.12-pre.1/unibox_0.12-pre.1_darwin_arm64.tar.gz"
      sha256 "9f906f01bf7760aca8ccf04cadc408888598db385e5e689c134041073ca54888"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.12-pre.1/unibox_0.12-pre.1_darwin_amd64.tar.gz"
      sha256 "97aec3f7c64bc3dfcd8846e9f4cd57501522db8721847bf7508066b6f214eac4"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/bitbyone/unibox/releases/download/v0.12-pre.1/unibox_0.12-pre.1_linux_arm64.tar.gz"
      sha256 "8fd7b00d122d2468c63d66936f8aa343885f9703e3acaea5d4d0545a5aa1203a"
    end
    on_intel do
      url "https://github.com/bitbyone/unibox/releases/download/v0.12-pre.1/unibox_0.12-pre.1_linux_amd64.tar.gz"
      sha256 "5f51fb40957705198fb1b335a016e6ba590d73e7c484ffe239edeb3434d83644"
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
