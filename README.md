# flashtui

A terminal UI for writing OS images to removable drives, with a guard on the
target device and optional headless first-boot setup.

```
┌─ IMAGES ─────────────┐┌─ DRIVE INFO ──────────────────────┐
│ raspios-trixie.img.xz││ /dev/sda   29.1 GiB  USB          │
│ archlinux.iso        ││ partition tbl  dos                │
└──────────────────────┘│  sda1  512 MiB  vfat  bootfs      │
┌─ TARGETS ────────────┐│    ████░░░░░░  13% · 439 MiB free │
│ + sda  STORAGE DEVICE││  sda2  28.6 GiB ext4  rootfs      │
│ ! nvme0n1  CT2000P5P ││ SELECTED IMAGE                    │
└──────────────────────┘│  fits, 26.3 GiB to spare          │
┌─ WRITE ──────────────────────────────────────────────────┐
│ writing  ████████░░░░  62%  1.7 GiB / 2.8 GiB            │
│          48.0 MB/s   elapsed 0:37   eta 0:23             │
└──────────────────────────────────────────────────────────┘
```

## What it does

- Writes raw, `.xz`, `.gz`, `.zst` and `.bz2` images, decompressing as it
  streams. Nothing is expanded to a temporary file first.
- Shows what is already on a drive — partition table, filesystems, labels,
  mount points and per-filesystem usage — so you can recognise the card you
  meant to pick before you overwrite it.
- Says whether the image fits, and by how much, before you start.
- Verifies by reading the device back and comparing a SHA-256 taken during the
  write.
- Configures a flashed image for headless first boot: user, password, SSH key,
  hostname and wifi.
- Mounts, unmounts and powers down drives.

## Install

```sh
make build          # produces ./flashtui
make check          # gofmt, vet and tests
```

Run it with `./flashtui`, or `./flashtui -dir /path/to/images` to start
somewhere other than `~/Downloads`.

## Keys

Navigation follows vim and lazygit.

| key | action |
| --- | --- |
| `j` `k` | move within a pane |
| `g` `G` | first / last |
| `ctrl+d` `ctrl+u` | half page |
| `tab` `shift+tab` | cycle panes |
| `h` `l` | previous / next pane |
| `1` `2` `3` | jump to images / targets / info |
| `f` | flash the selected image to the selected device |
| `m` `u` | mount / unmount every partition |
| `p` | unmount and power off, so it is safe to unplug |
| `o` | change the image directory |
| `r` | rescan |
| `v` | toggle read-back verification (on by default) |
| `s` | toggle headless setup after a successful write |
| `?` | help |
| `q` | quit |

## The guard

Pressing `f` does not write anything. It opens a confirmation showing the
device, its capacity, how it is connected, the filesystem labels already on it
and what will be destroyed — and asks you to type the target's bare device
name, for example `sda`.

A yes/no prompt is answered reflexively; typing `sdb` fails when you had `sda`
in mind. Matching is case sensitive and rejects the full `/dev/sda` form, both
of which are plausible mistypes.

A disk carrying `/`, `/boot` or `/home`, or one the kernel reports read-only,
cannot be confirmed at all, whatever you type.

## Privileges

flashtui runs unprivileged. Mounting, unmounting and powering off go through
udisks2, which handles removable media without a prompt on a desktop session.

Writing needs root, and is done with a single `pkexec` escalation per flash.
That is deliberate: polkit's `org.freedesktop.policykit.exec` action defaults
to `auth_admin` rather than `auth_admin_keep`, so nothing is cached and every
separate call prompts again. Writing, verifying and re-reading the partition
table as three escalations would put two extra password prompts in the middle
of a running flash.

If no polkit agent is running, `pkexec` prompts on the terminal instead. The
TUI hands the terminal over for the prompt and takes it back the moment the
privileged script reports that it is running, so the progress bar still covers
the whole write.

## Headless setup

With `s` enabled, a successful flash is followed by a form for the details
needed to reach the machine over the network without attaching a screen.

Only images that expose a first-boot mechanism can be configured, and flashtui
detects which it is rather than guessing from the filename:

| image | mechanism | supported |
| --- | --- | --- |
| Raspberry Pi OS | `userconf.txt`, `ssh` marker, ext4 root | yes |
| cloud-init image | NoCloud seed in `/var/lib/cloud/seed/nocloud-net` | yes |
| Arch / Ubuntu live ISO | none — read-only iso9660 | no, and it says why |

A live ISO's root is a read-only squashfs unpacked into RAM, so anything
written there is discarded on boot. flashtui reports that rather than
appearing to succeed.

Two details about Raspberry Pi OS are worth knowing, because getting either
wrong leaves an image that looks configured and is not reachable:

- The stock account ships with `/usr/sbin/nologin`. Only `userconf.txt` runs
  the `usermod -s /bin/bash` that unlocks it, so an SSH key on its own logs
  into nothing.
- The key is seeded under the stock home directory, because renaming the
  account on first boot runs `usermod -m -d`, which moves that directory along
  with it.

The SSH key field loads from `~/.ssh/*.pub` with `ctrl+k`. Only public keys are
read.

## Requirements

- Linux, with `udisks2`, `polkit` and util-linux.
- `mkpasswd` (from `whois`) or `openssl`, for SHA-512 password hashes.

flashtui prefers `/usr/bin/lsblk` over any `lsblk` earlier on `PATH`. A
Homebrew build is not linked against libudev and silently reports no filesystem
type, label or partition table, which would leave the drive readout and the
guard blank.

## Layout

| package | responsibility |
| --- | --- |
| `internal/blockdev` | enumerate disks, flag the system disk |
| `internal/imagefile` | find images, detect compression, resolve expanded size |
| `internal/flash` | write, progress, read-back verification |
| `internal/mountctl` | udisks2 mount / unmount / power off |
| `internal/seed` | detect image family, write first-boot configuration |
| `internal/privilege` | pkexec escalation |
| `internal/tui` | bubbletea model, panes, guard |
