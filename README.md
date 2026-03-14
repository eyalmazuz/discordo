# Discordo &middot; [![discord](https://img.shields.io/discord/1297292231299956788?color=5865F2&logo=discord&logoColor=white)](https://discord.com/invite/VzF9UFn2aB) [![ci](https://github.com/ayn2op/discordo/actions/workflows/ci.yml/badge.svg)](https://github.com/ayn2op/discordo/actions/workflows/ci.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/ayn2op/discordo)](https://goreportcard.com/report/github.com/ayn2op/discordo) [![license](https://img.shields.io/github/license/ayn2op/discordo?logo=github)](https://github.com/ayn2op/discordo/blob/master/LICENSE)

Discordo is a lightweight, secure, and feature-rich Discord terminal client. Heavily work-in-progress, expect breaking changes.

![Preview](.github/preview.png)

## Fork status

This repository is a fork of [ayn2op/discordo](https://github.com/ayn2op/discordo). It keeps the same overall terminal-client goal, but the feature set and testing depth in this fork have diverged from the upstream README.

The main additions in this fork are:

- custom emoji send/render fixes
- animated custom emoji playback while visible
- reaction picker on `+`
- per-chat message search popup on `Ctrl+f`
- improved DM unread highlighting and DM sorting refresh
- much deeper adjacent unit-test coverage across the repo

## Installation

### Prebuilt binaries

You can download and install a [prebuilt binary here](https://nightly.link/ayn2op/discordo/workflows/ci/main) for Windows, macOS, or Linux.

If you want the exact feature set documented in this fork, prefer building from this repository's source, since the upstream binary pipeline may not include fork-specific changes yet.

### Package managers

- Arch Linux: `yay -S discordo-git`
- Gentoo (available on the guru repos as a live ebuild): `emerge net-im/discordo`
- FreeBSD: `pkg install discordo` or via the ports system `make -C /usr/ports/net-im/discordo install clean`.
- Nix: Add `pkgs.discordo` to `environment.systemPackages` or `home.packages`.

- Windows (Scoop):

```sh
scoop bucket add vvxrtues https://github.com/vvirtues/bucket
scoop install discordo
```

### Building from source

```bash
git clone https://github.com/eyalmazuz/discordo
cd discordo
go build .
```

### Wayland clipboard support

`wl-clipboard` is required for clipboard support.

## Usage

### Token (UI, recommended)

1. Run the `discordo` executable with no arguments.

2. Enter your token and click on the "Login" button to save it.

### Token (environment variable)

Set the value of the `DISCORDO_TOKEN` environment variable to the authentication token to log in with.

```sh
DISCORDO_TOKEN="OTI2MDU5NTQxNDE2Nzc5ODA2.Yc2KKA.2iZ-5JxgxG-9Ub8GHzBSn-NJjNg" discordo
```

### QR (UI)

1. Run the `discordo` executable with no arguments.

2. Click on the "Login with QR" button.

3. Follow the instructions in the QR Login screen.

## Configuration

The configuration file allows you to configure and customize the behavior, keybindings, and theme of the application.

- Unix: `$XDG_CONFIG_HOME/discordo/config.toml` or `$HOME/.config/discordo/config.toml`
- Darwin: `$HOME/Library/Application Support/discordo/config.toml`
- Windows: `%AppData%/discordo/config.toml`

Discordo uses the default configuration if a configuration file is not found in the aforementioned path; however, the default configuration file is not written to the path. [The default configuration can be found here](./internal/config/config.toml).

> [!IMPORTANT]
> Automated user accounts or "self-bots" are against Discord's Terms of Service. I am not responsible for any loss caused by using "self-bots" or Discordo.

## Feature Status

The table below is intentionally practical rather than aspirational: it describes what this fork currently supports in the checked-in codebase.

| Discord workflow / feature | Status | Notes |
| --- | --- | --- |
| Token login | Implemented | UI login and `DISCORDO_TOKEN` environment-variable login are supported. |
| QR login | Implemented | Built-in QR login flow is available from the login UI. |
| Guild and channel navigation | Implemented | Guild tree, categories, text channels, DMs, and Group DMs are navigable from the sidebar. |
| Read and receive messages | Implemented | Message timeline, timestamps, unread markers, mentions, and live updates are supported. |
| Send messages | Implemented | Regular message sending works from the message input. |
| Reply to messages | Implemented | Reply and reply-with-mention workflows are supported from the messages list. |
| Edit and delete your messages | Implemented | Edit, delete, and delete-confirm flows are supported from the messages list. |
| Search inside the current chat | Implemented | `Ctrl+f` opens a popup search for the current channel / DM / thread and can jump to matching messages. |
| Add reactions | Implemented | `+` opens a searchable reaction picker for the selected message. |
| View reactions | Implemented | Unicode and custom reactions render inline with counts. |
| Custom emoji send/render | Implemented | Typing `:name:` can resolve to available custom emoji, and received custom emoji render inline. |
| Animated custom emoji | Implemented | Animated custom emoji play while visible on screen. |
| Inline image rendering | Implemented | Image attachments can render inline with `kitty` or half-block rendering. |
| Links and attachments | Implemented | Links and attachments can be opened from the selected message. |
| Paste, external editor, file picker | Implemented | Clipboard paste, `$EDITOR` / configured editor, and attachment picker workflows are supported. |
| Markdown rendering | Implemented | Mentions, URLs, attachments, emoji, and fenced code blocks are rendered. |
| Typing indicators | Implemented | Both sending and receiving typing indicators are configurable. |
| Desktop notifications | Implemented | Notification and sound support exist with per-config toggles. |
| Voice channels / stage channels | Partial | They appear in navigation, but full voice participation is not implemented. |
| Threads | Partial | Thread-like channel entries can be navigated, but full Discord thread parity is not complete. |
| Stickers | Not implemented | No full sticker picker / send / render workflow is documented in this fork. |
| GIF picker / Tenor search | Not implemented | No built-in GIF picker workflow is exposed in this fork today. |
| Voice calls / video / screen share | Not implemented | Real-time voice, video, and screen-share workflows are out of scope at present. |
| Polls / forum-post creation / activities | Not implemented | These Discord-native workflows do not have full client support in this fork. |

## Notable Shortcuts

Some useful workflows added or highlighted in this fork:

- `Ctrl+f`: search messages in the current chat
- `+`: open the reaction picker for the selected message
- `Ctrl+k`: open the generic picker
- `Ctrl+e`: open the configured external editor for the message input
- `Ctrl+\\`: open the attachment picker

The complete default keymap lives in [internal/config/config.toml](./internal/config/config.toml).

## Testing

This repository uses Go's normal convention for tests: unit tests live next to the code they exercise as `*_test.go` files instead of being kept in a separate `tests/` directory.

Common commands:

```bash
go test ./...
go test -coverprofile=/tmp/repo.cover.out ./...
go tool cover -func=/tmp/repo.cover.out
```

Current status in this fork:

- repo-wide unit-test statement coverage is `99.9%`
- tests cover core packages such as chat UI, markdown, image rendering, notifications, login, config, and picker behavior
- there is at least one checked-in fuzz target for markdown rendering stability

## Differences From Upstream

Compared with the upstream README, this fork currently documents and ships several behaviors that are specific to this repository:

- stronger test coverage and more regression coverage around chat flows
- in-chat message search popup
- reaction picker workflow
- custom emoji send/render fixes
- animated custom emoji support
- DM sidebar refresh improvements for unread ordering
- a standalone Kitty graphics diagnostic command under [cmd/kittytest](./cmd/kittytest)

## License

Copyright (C) 2025-present ayn2op

This project is licensed under the GNU General Public License v3.0 (GPL-3.0).
See the [LICENSE](./LICENSE) file for the full license text.
