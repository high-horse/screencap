screen for x11 but black screen in wayland
```bash
gst-launch-1.0 ximagesrc use-damage=0 ! videoconvert ! x264enc speed-preset=ultrafast tune=zerolatency ! mp4mux faststart=true fragment-duration=1000 ! filesink location=fast.mp4
```

captures cam in wayland
```bash
gst-launch-1.0 -e pipewiresrc ! videoconvert ! \
x264enc speed-preset=ultrafast tune=zerolatency ! \
mp4mux faststart=true fragment-duration=1000 ! \
filesink location=fast.mp4
```

captures cam in wayland
```bash
gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc speed-preset=ultrafast tune=zerolatency ! mp4mux faststart=true fragment-duration=1000 ! filesink location=fast.mp4
```

captures cam in wayland but media unplayable add `-e` at last to make it playable
```bash
gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc ! mp4mux ! filesink location=screen.mp4
```

captures cam in wayland
```bash
gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc tune=zerolatency speed-preset=ultrafast ! mp4mux ! filesink location=fast.mp4 -e
```