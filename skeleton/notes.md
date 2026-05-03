gst-launch-1.0 ximagesrc use-damage=0 ! videoconvert ! x264enc speed-preset=ultrafast tune=zerolatency ! mp4mux faststart=true fragment-duration=1000 ! filesink location=fast.mp4


gst-launch-1.0 -e pipewiresrc ! videoconvert ! \
x264enc speed-preset=ultrafast tune=zerolatency ! \
mp4mux faststart=true fragment-duration=1000 ! \
filesink location=fast.mp4

gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc speed-preset=ultrafast tune=zerolatency ! mp4mux faststart=true fragment-duration=1000 ! filesink location=fast.mp4


gst-launch-1.0 pipewiresrc ! videoconvert ! x264enc ! mp4mux ! filesink location=screen.mp4