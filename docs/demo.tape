# kpm terminal demo — from repository root, with kpm on PATH:
#   make build && export PATH="$(pwd)/bin:$PATH" && bash -c "vhs docs/demo.tape"
# Or: make install && bash -c "vhs docs/demo.tape"
# Use bash -c (not bare `vhs` from zsh) so the recorder does not inherit zsh/Oh My Zsh prompts.
# Re-record after EVERY VERSION bump — `kpm version` must show the release (e.g. v0.1.0).

Output docs/demo.gif

Require kpm

Set Shell "bash"
Set FontSize 14
Set Width 920
Set Height 560
Set Padding 20

Enter
Sleep 300ms

# Help
Type "kpm --help"
Enter
Sleep 2s

# Version (ldflags when built via make build / make install) — MUST match VERSION
Type "kpm version"
Enter
Sleep 1s

# Sample config (offline)
Type "kpm --print-sample-config | head -n 28"
Enter
Sleep 2s

# Core commands
Type "kpm sample --help"
Enter
Sleep 1.5s

Type "kpm backup --help"
Enter
Sleep 1.5s

Type "kpm export --help"
Enter
Sleep 1.5s

# Clean exit
Type "exit"
Enter
Sleep 500ms
