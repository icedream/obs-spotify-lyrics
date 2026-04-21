# WebSocket API

The lyrics widget backend exposes a single WebSocket endpoint that pushes
Spotify playback state and time-synced lyrics to every connected client.

This document is for anyone who builds their own widget HTML page, whether
connecting to the OBS plugin's built-in server or to the standalone `lyrics`
server executable.

## Endpoint

The API exposes its WebSocket on the path `/ws`, the full URL would be
`ws://<host>:<port>/ws`.

When using the **OBS plugin** the host is always `localhost`. The port is shown
in the plugin's settings panel (*Spotify Lyrics Config* → *Status*); by default
it is chosen automatically by the OS but it can be fixed to a specific port.

When using the **standalone server** the default `<host>:<port>` is
`localhost:8080` unless overridden with `--addr`.

## Connecting

Open a standard WebSocket connection to the `/ws` path. No subprotocol
negotiation or authentication is required.

```js
const ws = new WebSocket("ws://localhost:33331/ws");
```

The server immediately sends a **snapshot** of the current playback state the
moment the connection is established, so the widget is in sync right away
rather than waiting for the next Spotify poll.

The server never reads what you send. All traffic is one-directional:
**server → client only**. Do not send messages, they are silently discarded.

## Messages

Every message is a UTF-8 JSON text frame. Parse it with `JSON.parse`. The
`type` field tells you which kind of message it is.

### `"playing"`, track is active

Sent when a track is playing or paused.

```js
{
  "type": "playing",
  "is_playing": true,
  "position_ms": 43200,
  "track": {
    "id": "4PTG3Z6ehGkBFwjybzWkR8",
    "name": "Never Gonna Give You Up",
    "artists": [
      {
        "id": "0gxyHStUsqpMadRV0Di1Qt",
        "name": "Rick Astley"
      }
    ],
    "album": {
      "id": "6eUW0wxWtzkFdaEFsTJto6",
      "name": "Whenever You Need Somebody"
    },
    "duration_ms": 213573
  },
  "lyrics": [
    {
      "start_ms": 18187,
      "end_ms": 21400,
      "words": "We're no strangers to love"
    },
    {
      "start_ms": 21400,
      "end_ms": 24600,
      "words": "You know the rules and so do I"
    }
    // ...
  ]
}
```

| Field               | Type        | Description                                                                                                       |
| ------------------- | ----------- | ----------------------------------------------------------------------------------------------------------------- |
| `type`              | `"playing"` | Message discriminator.                                                                                            |
| `is_playing`        | boolean     | `true` while the track is actually advancing; `false` when paused.                                                |
| `position_ms`       | number      | Estimated playback position in milliseconds at the moment the message was sent.                                   |
| `track`             | object      | Track metadata (see below).                                                                                       |
| `track.id`          | string      | Spotify track ID.                                                                                                 |
| `track.name`        | string      | Track title.                                                                                                      |
| `track.artists`     | array       | One or more `{ id, name }` artist objects.                                                                        |
| `track.album`       | object      | `{ id, name }` album object.                                                                                      |
| `track.duration_ms` | number      | Total track duration in milliseconds.                                                                             |
| `lyrics`            | array       | Time-synced lyric lines (see below). May be empty if lyrics are not yet available or do not exist for this track. |
| `lyrics[].start_ms` | number      | Millisecond timestamp at which this line starts.                                                                  |
| `lyrics[].end_ms`   | number      | Millisecond timestamp at which this line ends (i.e. the next line begins, or the track ends).                     |
| `lyrics[].words`    | string      | Lyric text for this line.                                                                                         |

### `"idle"`, nothing is playing

Sent when Spotify reports no active playback (stopped, no device active, or a
non-track item like a podcast episode).

```json
{
  "type": "idle"
}
```

## When messages are sent

| Event                       | Message sent                                                                                                                               |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Client connects             | Snapshot of current state (`"playing"` or `"idle"`) with `position_ms` estimated to the current moment.                                    |
| Track changes               | Immediate `"playing"` with the new track and **empty `lyrics`**; a second `"playing"` follows shortly after once lyrics have been fetched. |
| Lyrics become available     | `"playing"` with `lyrics` populated (same track, same position).                                                                           |
| Playback paused or resumed  | `"playing"` with updated `is_playing`.                                                                                                     |
| Seek detected (>1.5 s jump) | `"playing"` with updated `position_ms`.                                                                                                    |
| Playback stops              | `"idle"`.                                                                                                                                  |

> **Note on `position_ms`:** The server polls Spotify every ~3 s while
> playing and every ~5 s while idle. Between polls it extrapolates the position
> using a local clock anchor, so the value in each message is as accurate as
> possible without over-polling. Your widget should also extrapolate locally
> (e.g. with `requestAnimationFrame` or a `setInterval`) to keep the active
> lyric line highlighted smoothly between messages.

## HTML/JS example

```html
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
  </head>
  <body>
    <div>
      <span id="state">loading</span>
      <a id="currently-playing"></a>
    </div>
    <div id="line"></div>
    <script>
      let lyrics = [];
      let startedAt = 0; // local timestamp when position_ms was last set

      /**
       * Called once document is done loading
       */
      function onLoad() {
        // Connect to the lyrics server
        document.getElementById("state").innerText = "connecting";
        const ws = new WebSocket("ws://localhost:33331/ws");

        // Handle new track/lyrics info
        ws.addEventListener("message", (event) => {
          // This is our message
          const msg = JSON.parse(event.data);

          // Show "idle" or "playing" on the page
          document.getElementById("state").innerText = msg.type;

          if (msg.type === "idle") {
            // Empty everything else, it should just say "idle" and that's it
            lyrics = [];
            document.getElementById("currently-playing").innerText = "";
            document.getElementById("currently-playing").href = "";
            document.getElementById("line").textContent = "";
            return;
          }

          // From here on a track is at least loaded, if not playing.
          
          // Show track information and link to Spotify
          if (msg.track) {
            document.getElementById("currently-playing").innerText =
              msg.track.artists.map(a => a.name).join(', ') + " - " + msg.track.name;
            document.getElementById("currently-playing").href =
              "https://open.spotify.com/track/" + msg.track.id;
          }
          
          // Store the lyric lines if any - the lyrics server may send the new
          // track info without lyrics on transition, in that case we keep the
          // old lyrics for now
          if (msg.lyrics.length) {
            lyrics = msg.lyrics;
          }

          // Record local start timestamp anchor, from this we can derive
          // current position in song to show lyric for
          startedAt = performance.now() - msg.position_ms;
          if (!msg.is_playing) startedAt = null;
        });

        // Start rendering lyrics to the screen!
        requestAnimationFrame(tick);
      }

      /**
       * Called on each frame to update the current lyric line.
       */
      function tick() {
        // On next frame, call this again for continuous updating
        requestAnimationFrame(tick);

        // Not playing/no track/no lyrics? Skip!
        if (!lyrics.length || startedAt === null) return;

        // Check where in the track we should be and show matching lyric for it
        const pos = performance.now() - startedAt;
        const active = lyrics.findLast((l) => l.start_ms <= pos);
        document.getElementById("line").textContent = active ? active.words : "";
      }

      window.addEventListener("load", onLoad);
    </script>
  </body>
</html>
```
