# Arduino Flasher CLI

A tool to download and flash Debian images on the board.

## Build and test it locally

Build it with `task arduino-flasher-cli:build` and run `task flash:staging` to download and flash the latest image from staging.

Alternatively, these are the steps to test it locally:

1. download a debian image release (for example from [here](https:/downloads.oniudra.cc/debian-im/Stable/20250902-166/arduino-unoq-debian-image-20250902-166.tar.xz), then create the following structure under `arduino-flasher-cli/`:

```
arduino-flasher-cli/public/
└── debian-im
    └── Stable
        ├── 20250902-166
        │   └── arduino-unoq-debian-image-20250902-166.tar.xz
        └── info.json
```

2. Populate `info.json`:

```
{
  "latest": {
    "version": "20250902-166",
    "url": "http://127.0.0.1:3001/debian-im/Stable/20250902-166/arduino-unoq-debian-image-20250902-166.tar.xz",
    "md5sum": "ad0aac0a9b18982e9dce0dd99808a5e5"
  }
}
```

3. `task debian:release-server:start`
4. `UPDATE_URL=http://127.0.0.1:3001 ./build/arduino-flasher-cli flash latest`

## Flash from staging

To download and flash the latest image from staging, the tool needs cloudflare credentials. This is the command that should be used:

```sh
CF_ACCESS_CLIENT_ID="$(aws ssm get-parameter --profile arduino-staging --with-decryption --name /devops/downloads/cloudflare_access_client_id --query Parameter.Value --output text)" CF_ACCESS_CLIENT_SECRET="$(aws ssm get-parameter --profile arduino-staging --with-decryption --name /devops/downloads/cloudflare_access_client_secret --query Parameter.Value --output text)" ./arduino-flasher-cli flash latest
```
