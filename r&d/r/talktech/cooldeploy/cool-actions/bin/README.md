# bin

Empty until you fill it. `setup-coolctl` uses the `coolctl` on the runner
image when there is one. For runners without it, bundle the binary here:

```sh
make bundle COOLCTL_SRC=../coolctl
```

writes `coolctl-linux-amd64`, `coolctl-linux-arm64` and `SHA256SUMS`. Commit
them before tagging a release; the action checks the checksum before it
runs the binary.
