---
name: video-encrypt
description: Encrypt a video file with the encv-go video plugin. Use this skill whenever the user asks to lock / encrypt / protect a video file (mp4, mkv, avi, mov, flv, wmv, ts, m4v, webm).
---

# Video Encrypt

## When to use

Activate this skill as soon as the user wants to encrypt a
video file. Do NOT attempt to handle the file with raw
filesystem tools; the encryption MUST go through the
`video_encrypt` registered tool so the container version,
password strategy, and post-processing pipeline are all
honoured.

## Tool call

Always invoke `video_encrypt` with the following shape:

```json
{
  "path": "<absolute path the user provided>",
  "password": "<the user's password>",
  "container_version": "<from agent_settings.default_container_version>"
}
```

If `agent_settings.default_container_version` is unset, omit
`container_version` and let the tool fall back to its
built-in default.

## Password strategy

* If the user supplied a password, pass it through verbatim.
* If the user did not supply one, fall back to
  `agent_settings.global_password`.
* If neither is set, ask the user once before calling the
  tool. Do not retry silently.

## Success / failure handling

* On success, the tool returns the path to the
  encrypted container file (extension is decided by
  `plugin.GetContainerExtension()` — typically `.sccgv` for video,
  `.sccga` for audio, `.sccgt` for text, etc.). Report the path back to
  the user and suggest running `video_decrypt` if they ever need to
  recover the original.
* On failure, surface the error message verbatim and stop
  the turn — never retry the same call with the same args.
