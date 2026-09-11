# B3 third-party `.gosig` load module

Used only to regenerate Hyperliquid-adjacent stubs:

```bash
cd testdata/sdk-b3
go mod download
# from repo root, with this dir as cwd so packages.Load finds go.mod:
goop get-go-sig github.com/gorilla/websocket
# then copy $GOOP_HOME/build/go-sigs/*.gosig → ../../goop-sigs/ and trim
```

Committed overrides live in `goop-sigs/github_com_*.gosig`. They are **not**
added to the compiler `CuratedPackages` list (go-ethereum would own CI).
