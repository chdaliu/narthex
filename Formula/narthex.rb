# Master copy of the Homebrew formula: sync into the homebrew-tap repo
# (github.com/chdaliu/homebrew-tap) after each release, or install locally with
# `brew install --build-from-source ./Formula/narthex.rb`.
# sha256 placeholders: fill from the release SHA256SUMS.
class Narthex < Formula
  desc "Lightweight web dashboard to launch and stop local apps"
  homepage "https://github.com/chdaliu/narthex"
  license "MIT"
  version "0.1.0" # keep in sync with the latest release tag

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
    strategy :github_latest
  end

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_arm64.tar.gz"
      sha256 "a8246a311dfa5cf1f48abcd36eb3ff1807001149914fd50e4d999d2106c45de4"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_amd64.tar.gz"
      sha256 "ca73991002f4e9b0868ef7232a467299e30fb3886bac3389128efe5d2928421d"
    end
  end
  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_arm64.tar.gz"
      sha256 "352a751a6797f335687e1c52decc241e4c43557d1b63dcc627077464d2325c7f"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_amd64.tar.gz"
      sha256 "7c72ba67127a5bd3690917ed61efdf6cc0d855ee28d33d22b074261ff2f072ff"
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
