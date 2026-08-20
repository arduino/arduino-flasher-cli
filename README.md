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

## Update QDL version

flasher-cli embeds a statically builded [qdl](https://github.com/linux-msm/qdl) binary taken from the https://github.com/arduino/qdl-packing repo. To update the qdl version you need to:

1. Make sure the qdl-packing repo has the desired version of qdl (check the release page [here](https://github.com/arduino/qdl-packing/releases)) if not, create a PR to update it.

2. Update the `TAG` variable in script `internal/artifacts/download_resources.sh` to the desired version.

3. Test that everything works by running `task clean build` and testing the resulting binary.

4. Create a new release of flasher-cli, the build process will automatically download and embed the new qdl binary.
