# Arduino Flasher CLI

A tool to download and flash Debian images on the board.

## Build and test it locally

Build it with `task arduino-flasher-cli:build` and run:

```sh
# Flash the latest release of the Debian image
./build/arduino-flasher-cli flash latest

# Flash a local image. It works with either an archived or extracted image
./build/arduino-flasher-cli flash path/to/downloaded/image
```
