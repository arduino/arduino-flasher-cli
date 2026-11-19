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

## Download images

`arduino-flasher-cli download unoq` saves the image archive in the user cache,
under `arduino-flasher-cli/images/`. If the cache already has the image, the
download is skipped. An interrupted download resumes on the next run.

Use `--dest-dir` to download to an existing directory instead of the cache:

```sh
arduino-flasher-cli download unoq --dest-dir .
```

## Reuse images when flashing

`arduino-flasher-cli flash unoq` uses the cached image if there is one.
Otherwise it downloads the image to a temporary directory and removes it after
the flash.

Use `--cache` to keep the downloaded image in the cache for future flashes:

```sh
arduino-flasher-cli flash unoq --cache
```

In interactive mode, cached images are marked `(cached)`, and the wizard asks
whether to keep new downloads. `--temp-dir` selects where the image is
extracted. Local image files and directories are flashed directly and are not
added to the cache.

## Manage cached images

Inspect or clear the image cache with:

```sh
arduino-flasher-cli cache list
arduino-flasher-cli cache list --format json
arduino-flasher-cli cache clear
```

`cache list` shows the file name, size and download date of each cached image.
It works offline. `cache clear` removes all cached images and
incomplete downloads. Files saved with `--dest-dir` are not removed.

The `list` command marks cached images in the `CACHED` column, and with `cached`
in JSON output.

## Cache behavior over gRPC

The `Flash` RPC uses the same image cache as the CLI. Set `FlashRequest.cache`
to `true` to keep new downloads in the cache. The default is `false`, as for
`flash` without `--cache`.

`FlashRequest.temp_path` selects the parent of the temporary directory for the
download and the extraction. If it is empty, the user cache directory is used.
The temporary directory is removed after the flash, also on failure or
cancellation.

Download start/end events are also sent on a cache hit, with a completed progress
update and a `Using cached image` end message. Extraction and flash progress use
the existing response messages.

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
