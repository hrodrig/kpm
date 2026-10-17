# Homebrew tap skeleton — kpm
#
# `kpm.rb` is a working formula for the **v0.8.0** release: real URLs, real SHA-256s
# from that release's checksums.txt. It is not installed anywhere yet because the tap
# repository does not exist.
#
# To publish it (the part that needs a human, because it touches GitHub):
#
#   1. Create the tap repo:  hrodrig/homebrew-kpm   (public)
#      `brew tap hrodrig/kpm` resolves to it, and `brew install hrodrig/kpm/kpm`
#      then works.
#   2. Copy `kpm.rb` into `Formula/kpm.rb` in that repo.
#   3. Bump it on every release: the four SHA-256s come from
#      `https://github.com/hrodrig/kpm/releases/download/v<X.Y.Z>/checksums.txt`.
#
# Automation, when you want it: GoReleaser in this repo supports the homebrew
# publisher (`goreleaser release --skip homebrew` proves it), so a `brews:` block
# plus a `HOMEBREW_TAP_TOKEN` secret with write access to the tap turns step 3 into
# part of the release. That needs the token, which is why it is not wired up here.
#
# Verify a formula before pushing it:
#
#   brew install --build-from-source ./contrib/homebrew/kpm.rb
#   brew test kpm
#   brew audit --strict --new kpm
