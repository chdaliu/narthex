# Master copy of the Homebrew formula: sync into the homebrew-tap repo
# (github.com/chdaliu/homebrew-tap) after each release, or install locally with
# `brew install --build-from-source ./Formula/narthex.rb`.
# sha256 placeholders: fill from the release SHA256SUMS.
class Narthex < Formula
  desc "Lightweight web dashboard to launch and stop local apps"
  homepage "https://github.com/chdaliu/narthex"
  license "MIT"
  version "0.0.1" # keep in sync with the latest release tag

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
    strategy :github_latest
  end

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
  end
  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
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
