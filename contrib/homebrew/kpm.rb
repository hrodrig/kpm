class Kpm < Formula
  desc "Kubernetes pod CPU and memory metrics into SQLite, with offline rightsizing"
  homepage "https://github.com/hrodrig/kpm"
  version "0.8.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/hrodrig/kpm/releases/download/v#{version}/kpm_v#{version}_darwin_arm64.tar.gz"
      sha256 "3fafe9db8bef8f804829834039137aa6afa59f2d6d8a04cf48a6133e0fbec2a7"
    end
    on_intel do
      url "https://github.com/hrodrig/kpm/releases/download/v#{version}/kpm_v#{version}_darwin_amd64.tar.gz"
      sha256 "96e2e107121c4da176036f346868166723207c42b1d5c1777b70f9b25d481c89"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/hrodrig/kpm/releases/download/v#{version}/kpm_v#{version}_linux_arm64.tar.gz"
      sha256 "f74847485ffd507e6b369f75b7efcdbae35bd56d31cc97c69e54dfff22b86748"
    end
    on_intel do
      url "https://github.com/hrodrig/kpm/releases/download/v#{version}/kpm_v#{version}_linux_amd64.tar.gz"
      sha256 "e5936ea453dbaeb0ead129086f48059a89d111568141b66aa29ae61e899786b3"
    end
  end

  def install
    bin.install "kpm"
    # kubectl plugin shim: `kubectl kpm ...` works once this is on PATH.
    bin.install "kubectl-kpm"
    man1.install "share/man/man1/kpm.1"
    man1.install "share/man/man1/kubectl-kpm.1"
    (share/"doc/kpm").install "share/doc/kpm/LICENSE"
    (share/"kpm").install "share/examples/kpm/kpm.sample.yml"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kpm version")
  end
end
