# Standalone replay converter

From the Go module directory (`src/`), use the repository's pinned Go toolchain.

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o goship-replay-convert ./cmd/replay-convert
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o goship-replay-convert.exe ./cmd/replay-convert
./goship-replay-convert --input "Previous mission.g2r" --output "Converted mission.g2r"
```

The output must be a new path. Conversion rereads every operation and compares the original launch, checkpoints, events and ending before publication. Exit status 0 means success, 1 means conversion or filesystem failure and 2 means invalid arguments. Use SETUP for in-place library upgrades with backups.
