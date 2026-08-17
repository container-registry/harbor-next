class HarborCli < Formula
  desc "CLI for Harbor container registry"
  homepage "https://github.com/goharbor/harbor-cli"
  url "https://github.com/goharbor/harbor-cli/archive/refs/tags/v0.0.23.tar.gz"
  sha256 "7afe257e071c509143fe9f2b4b7bbe16e8076e42001b07a35161160f77697c30"
  license "Apache-2.0"

  bottle do
    root_url "https://ghcr.io/v2/homebrew/core"
    sha256 cellar: :any_skip_relocation, arm64_tahoe:   "dbae3e9109e4a5c952a01a9543d3317bfec4ee614bc9209b4a6ff6f28061b1e1"
    sha256 cellar: :any_skip_relocation, arm64_sequoia: "5be6ea23753c5bbe04fcf9bfccde2dcdf58be90d22b39097d0ecf364310b8381"
    sha256 cellar: :any_skip_relocation, arm64_sonoma:  "2285ef1389552686bebbe0641fbc28c0f1ece7072c423ee5af9827224c20c1a7"
    sha256 cellar: :any_skip_relocation, sonoma:        "9cf6c49d370f2cfbc7f5cdd67ab8543f3d109cfa3bb3feb21ab7b1e69f311dc8"
    sha256 cellar: :any_skip_relocation, arm64_linux:   "24c6f674dcc6ea1be18e9378ce0721ceee62a4957e4fe17eb73d2ca5e84cf3d6"
    sha256 cellar: :any,                 x86_64_linux:  "bc4e83560d87c2cee14d658308700d2106f9b43d506e813ff196ee349cc7887d"
  end

  depends_on "go" => :build

  def install
    ldflags = %W[
      -s -w
      -X github.com/goharbor/harbor-cli/cmd/harbor/internal/version.Version=#{version}
      -X github.com/goharbor/harbor-cli/cmd/harbor/internal/version.GoVersion=#{Formula["go"].version}
      -X github.com/goharbor/harbor-cli/cmd/harbor/internal/version.GitCommit=homebrew-fixture
      -X github.com/goharbor/harbor-cli/cmd/harbor/internal/version.BuildTime=#{time.iso8601}
    ]
    system "go", "build", *std_go_args(ldflags:, output: bin/"harbor"), "./cmd/harbor"

    generate_completions_from_executable(bin/"harbor", shell_parameter_format: :cobra)
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/harbor version")
  end
end
