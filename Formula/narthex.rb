# Master copy of the Homebrew formula: sync into the homebrew-tap repo
# (github.com/chdaliu/homebrew-tap) after each release, or install locally with
# `brew install --build-from-source ./Formula/narthex.rb`.
# sha256 placeholders: fill from the release SHA256SUMS.
class Narthex < Formula
  desc "Lightweight web dashboard to launch and stop local apps"
  homepage "https://github.com/chdaliu/narthex"
  license "MIT"
  version "0.1.1" # keep in sync with the latest release tag

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
    strategy :github_latest
  end

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_arm64.tar.gz"
      sha256 "94ce9b89abb375dbad988680b39da01d1a149bcdd23748f94ba7930d5cf6437a"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_amd64.tar.gz"
      sha256 "de24b73e244a992dde00f1cd2fa8346c94b6fb7b8d1c9f95d07c66da1d25cfc7"
    end
  end
  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_arm64.tar.gz"
      sha256 "5632a67506e3e851b37be29ba83a6bf5d28f33cdefd937c637dca77146d3dfa0"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_amd64.tar.gz"
      sha256 "f652489e9ea56fee04aba9e3817cb87f17ba1090177b3a46ee95c42413765f82"
    end
  end

  # The service block makes `brew services start narthex` available as an
  # alternative to the built-in `narthex autostart install` launchd job.
  # narthex detects the brew job (homebrew.mxcl.narthex plist) and disables
  # its own toggle, so the two mechanisms never double-start serve.
  service do
    run [opt_bin/"narthex", "serve"]
    keep_alive true
  end

  def install
    bin.install "narthex"
  end

  def caveats
    <<~EOS
      First run creates the config and prints a random password:
        narthex serve
      Auto-start at login — pick ONE:
        narthex autostart install
        brew services start narthex
      The dashboard's start-at-login toggle disables itself while the
      brew service is active. After a `brew upgrade`, re-run
      `narthex autostart install` if you use the built-in job
      (the launchd job points at the old Cellar path).
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/narthex version")
  end
end
