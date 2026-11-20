# Arduino Flasher CLI

A tool to download and flash Debian images on the board.

## Docs

For a full guide on how to use it, see the [User documentation](https://docs.arduino.cc/tutorials/uno-q/update-image/).

## Build and test it locally

Build it with `task build` and run:

```sh
# Flash the most recent Debian image for the UNO Q
./build/arduino-flasher-cli flash unoq

# Flash a local image. It works with either an archived or extracted image
./build/arduino-flasher-cli flash path/to/downloaded/image
```

## Image store

The flasher keeps its files in one folder, `arduino-flasher-cli/` in the user
cache directory. The CLI and the gRPC daemon share it:

- `images/` holds the kept images. A flash never removes them.
- `images/.partial/` holds downloads that are not kept. An interrupted download
  resumes there, and a failed flash leaves its image there for a retry.
- A flash extracts the image into a temporary folder, by default in
  `arduino-flasher-cli/`, and removes it on every exit.

A flash reuses a stored image (kept or partial) after it checks the checksum.

`download` saves images in the image store, under the user cache directory in
`arduino-flasher-cli/images/`. A stored image is reused, and an interrupted
download resumes on the next run. Use `--dest-dir` to save the image in another
directory instead.

`flash` uses a stored image if there is one. Use `--keep` to keep a new
download in the image store. Without it, `flash` removes the download after a
successful flash. An interrupted download resumes, and a failed flash keeps the
download, so a retry does not download it again. `list` marks kept images in
the `KEPT` column, and the interactive mode marks them `(kept)`.

`images list` also shows the size of the partial downloads and of the other
files that the flasher left, such as the directories of a crashed flash.
`prune` removes everything except the kept images, and `images remove` removes
kept images.

```sh
arduino-flasher-cli images list
arduino-flasher-cli images remove --all
arduino-flasher-cli prune
```

Over gRPC, the same rules apply. Set `FlashRequest.keep` to `true` to keep a
new download in the image store. `List` sets `Release.kept` for kept images.
`FlashRequest.temp_path` selects the parent of the temporary extraction
directory, which is removed after the flash. When the image is already stored,
the download events report a completed download with the message
`Using stored image`.

## Read an additional index

Only the published indexes are read by default. `ARDUINO_FLASHER_ADDITIONAL_URLS`
adds more, separated by commas or newlines, and they are read _alongside_ the
published ones rather than instead of them:

```sh
ARDUINO_FLASHER_ADDITIONAL_URLS="http://127.0.0.1:3001/debian-im/Stable/info.json" \
  ./build/arduino-flasher-cli list
```

`ARDUINO_FLASHER_ADDITIONAL_HEADERS` adds HTTP headers, one `Name: value` per
line, sent both when reading an index and when downloading the image it points
at, for a host that asks to be authenticated:

```sh
ARDUINO_FLASHER_ADDITIONAL_HEADERS="Authorization: Bearer $TOKEN" \
  ./build/arduino-flasher-cli list
```

An index that cannot be read is reported as a warning and the others are still
listed. One that is not published yet is passed over in silence.

## Update QDL version

flasher-cli embeds a statically builded [qdl](https://github.com/linux-msm/qdl) binary taken from the https://github.com/arduino/qdl-packing repo. To update the qdl version you need to:

1. Make sure the qdl-packing repo has the desired version of qdl (check the release page [here](https://github.com/arduino/qdl-packing/releases)) if not, create a PR to update it.

2. Update the `TAG` variable in script `internal/artifacts/download_resources.sh` to the desired version.

3. Test that everything works by running `task clean build` and testing the resulting binary.

4. Create a new release of flasher-cli, the build process will automatically download and embed the new qdl binary.
