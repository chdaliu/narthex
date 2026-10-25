# Master copy of the Homebrew formula: sync into the homebrew-tap repo
# (github.com/chdaliu/homebrew-tap) after each release, or install locally with
# `brew install --build-from-source ./Formula/narthex.rb`.
# sha256 placeholders: fill from the release SHA256SUMS.
class Narthex < Formula
  desc "Lightweight web dashboard to launch and stop local apps"
  homepage "https://github.com/chdaliu/narthex"
  license "MIT"
  version "0.2.0" # keep in sync with the latest release tag

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
    strategy :github_latest
  end

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_arm64.tar.gz"
      sha256 "caffe965d66256596e77b96d5e9e0db9cd07c05d10d461a9fc9000ec6266a966"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_darwin_amd64.tar.gz"
      sha256 "bc209ae2efef3884415a708ecc6653e87e822b5c54cc8fffa7362ce6d34eae84"
    end
  end
  on_linux do
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_arm64.tar.gz"
      sha256 "02242fac51333fb99df9a8484116ac69673edd61faa87918def2496d1dc877a9"
    else
      url "https://github.com/chdaliu/narthex/releases/download/v#{version}/narthex_#{version}_linux_amd64.tar.gz"
      sha256 "232ce0c9170be4446c1a3429330763f450fb941e3b341302dce5960fffebeec6"
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
