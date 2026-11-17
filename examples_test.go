package aiff

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func ExampleDecoder_Duration() {
	path, _ := filepath.Abs("fixtures/kick.aif")
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	c := NewDecoder(f)
	d, _ := c.Duration()
	fmt.Printf("kick.aif has a duration of %f seconds\n", d.Seconds())
	// Output:
	// kick.aif has a duration of 0.203356 seconds
}

func ExampleDecoder_IsValidFile() {
	f, err := os.Open("fixtures/kick.aif")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	fmt.Printf("is this file valid: %t", NewDecoder(f).IsValidFile())
	// Output: is this file valid: true
}

func ExampleDecoder_ChannelLabels() {
	f, err := os.Open("fixtures/ring.aif")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	d := NewDecoder(f)
	d.ReadInfo()
	fmt.Printf("layout: %s\n", d.ChannelLayout.Tag)
	fmt.Printf("channels: %v\n", d.ChannelLabels())
	// Output:
	// layout: Stereo
	// channels: [Left Right]
}

func ExampleChannelLayout() {
	// Layouts can be built from a predefined tag, a channel bitmap or an
	// explicit list of channel labels.
	tag := NewChannelLayoutFromTag(ChannelLayoutTagMPEG_5_1_A)
	bitmap := NewChannelLayoutFromBitmap(ChannelBitLeft | ChannelBitRight)
	explicit := NewChannelLayoutFromLabels(ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight)

	fmt.Println(tag.Labels())
	fmt.Println(bitmap.Labels())
	fmt.Println(explicit.Labels())
	// Output:
	// [Left Right Center LFE LeftSurroundDirect RightSurroundDirect]
	// [Left Right]
	// [Center Left Right]
}
