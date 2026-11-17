# aiff codec

[![GoDoc](https://godoc.org/github.com/go-audio/aiff?status.svg)](https://godoc.org/github.com/go-audio/aiff)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-audio/aiff)](https://goreportcard.com/report/github.com/go-audio/aiff)
[![Coverage Status](https://codecov.io/gh/go-audio/aiff/graph/badge.svg)](https://codecov.io/gh/go-audio/aiff)
[![Build Status](https://travis-ci.org/go-audio/aiff.svg)](https://travis-ci.org/go-audio/aiff)

Support for int PCM data wrapped in aiff format.
AIFC/sowt format supported.

Channel order and labels are read from (and written to) the `CHAN` chunk.
Layouts can be predefined tags, channel bitmaps or explicit channel
descriptions; see `Decoder.ChannelLabels` and `Encoder.ChannelLayout`.

See [GoDoc](https://godoc.org/github.com/go-audio/aiff) for more details and examples.
