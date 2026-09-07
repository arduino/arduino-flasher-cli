# arduino-flasher-cli licensing information

## Main product license

- Copyright: Arduino s.r.l. and/or its affiliated companies
- License: GPL-3.0-or-later (`LICENSE`, `LICENSES/GPL-3.0-or-later.txt`)

## Third-party notices

`arduino-flasher-cli` ships as a single self-contained executable. It does not
require you to install any external flashing tool: the tools it needs are
**embedded into the binary at build time** and are extracted to a temporary
directory when a command runs. The list below records what is embedded and
under which license.

### qdl

- Upstream: <https://github.com/linux-msm/qdl>
- Copyright: 2016-2017 Linaro Ltd.; 2016 Bjorn Andersson <bjorn@kryo.se>
- License: BSD-3-Clause

The command-line tool that performs the actual flashing over USB in EDL mode.
Static builds are produced by <https://github.com/arduino/qdl-packing> and one
per target platform is embedded into the executable. The binaries are not kept
in this repository; they are downloaded during the build by
`internal/updater/artifacts/download_resources.sh`.

### Windows driver files

- Files: `cmd/arduino-flasher-cli/drivers/src/unoq.cat`, `unoq.inf`
- Copyright: Arduino s.r.l. and/or its affiliated companies
- License: GPL-3.0-or-later

Embedded in the Windows build only, and installed with `pnputil` when the
`flash` command runs.
