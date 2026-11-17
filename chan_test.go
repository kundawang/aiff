package aiff

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/go-audio/audio"
)

// chanBytes serializes an AudioChannelLayout the way it must appear in a CHAN
// chunk payload.
func chanBytes(l *ChannelLayout) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint32(l.Tag))
	binary.Write(buf, binary.BigEndian, uint32(l.Bitmap))
	binary.Write(buf, binary.BigEndian, uint32(len(l.Descriptions)))
	for _, d := range l.Descriptions {
		binary.Write(buf, binary.BigEndian, uint32(d.Label))
		binary.Write(buf, binary.BigEndian, d.Flags)
		for _, c := range d.Coordinates {
			binary.Write(buf, binary.BigEndian, math.Float32bits(c))
		}
	}
	return buf.Bytes()
}

// makeAIFF builds a minimal AIFF file with an optional list of extra chunks
// placed between COMM and SSND. Each extra chunk is (id, payload).
func makeAIFF(numChans int, bitDepth int, frames [][]int, extras [][2]interface{}) []byte {
	buf := new(bytes.Buffer)

	body := new(bytes.Buffer)
	// COMM
	binary.Write(body, binary.BigEndian, COMMID)
	binary.Write(body, binary.BigEndian, uint32(18))
	binary.Write(body, binary.BigEndian, uint16(numChans))
	binary.Write(body, binary.BigEndian, uint32(len(frames)))
	binary.Write(body, binary.BigEndian, uint16(bitDepth))
	sampleRateBytes := audio.IntToIEEEFloat(44100)
	body.Write(sampleRateBytes[:])

	for _, extra := range extras {
		id := extra[0].([4]byte)
		payload := extra[1].([]byte)
		body.Write(id[:])
		binary.Write(body, binary.BigEndian, uint32(len(payload)))
		body.Write(payload)
		if len(payload)%2 != 0 {
			body.WriteByte(0)
		}
	}

	// SSND
	binary.Write(body, binary.BigEndian, SSNDID)
	pcmLen := len(frames) * numChans * (bitDepth / 8)
	binary.Write(body, binary.BigEndian, uint32(pcmLen+8))
	binary.Write(body, binary.BigEndian, uint32(0)) // offset
	binary.Write(body, binary.BigEndian, uint32(0)) // block size
	for _, frame := range frames {
		for _, s := range frame {
			switch bitDepth {
			case 16:
				binary.Write(body, binary.BigEndian, int16(s))
			case 24:
				body.Write(audio.Int32toInt24BEBytes(int32(s)))
			default:
				panic("unsupported test bit depth")
			}
		}
	}
	if pcmLen%2 != 0 {
		body.WriteByte(0)
	}

	binary.Write(buf, binary.BigEndian, formID)
	binary.Write(buf, binary.BigEndian, uint32(body.Len()+4))
	binary.Write(buf, binary.BigEndian, aiffID)
	buf.Write(body.Bytes())
	return buf.Bytes()
}

func decodeBytes(t *testing.T, data []byte) (*Decoder, *audio.IntBuffer) {
	t.Helper()
	d := NewDecoder(bytes.NewReader(data))
	buf, err := d.FullPCMBuffer()
	if err != nil {
		t.Fatalf("FullPCMBuffer failed: %v", err)
	}
	if err := d.Drain(); err != nil {
		t.Fatalf("Drain failed: %v", err)
	}
	return d, buf
}

func encodeBytes(t *testing.T, numChans, bitDepth int, layout *ChannelLayout, frames [][]int) []byte {
	t.Helper()
	var data []int
	for _, frame := range frames {
		data = append(data, frame...)
	}
	out := &memWriteSeeker{}
	e := NewEncoder(out, 44100, bitDepth, numChans)
	e.ChannelLayout = layout
	buf := &audio.IntBuffer{
		Format: &audio.Format{NumChannels: numChans, SampleRate: 44100},
		Data:   data,
	}
	if err := e.Write(buf); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	return out.Bytes()
}

type memWriteSeeker struct {
	buf []byte
	pos int
}

func (m *memWriteSeeker) Write(p []byte) (int, error) {
	if need := m.pos + len(p); need > len(m.buf) {
		grown := make([]byte, need)
		copy(grown, m.buf)
		m.buf = grown
	}
	copy(m.buf[m.pos:], p)
	m.pos += len(p)
	return len(p), nil
}

func (m *memWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var base int
	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = m.pos
	case io.SeekEnd:
		base = len(m.buf)
	}
	pos := base + int(offset)
	if pos < 0 {
		return 0, errors.New("negative position")
	}
	if pos > len(m.buf) {
		m.buf = append(m.buf, make([]byte, pos-len(m.buf))...)
	}
	m.pos = pos
	return int64(pos), nil
}

func (m *memWriteSeeker) Bytes() []byte { return m.buf }

var fiveOneFrames = [][]int{
	{1, 2, 3, 4, 5, 6},
	{10, 20, 30, 40, 50, 60},
	{100, 200, 300, 400, 500, 600},
}

var threeChanFrames = [][]int{
	{11, 22, 33},
	{-11, -22, -33},
	{1, 2, 3},
}

func TestChanChunkLayoutTag(t *testing.T) {
	layout := NewChannelLayoutFromTag(ChannelLayoutTagMPEG_5_1_A)
	data := makeAIFF(6, 16, fiveOneFrames, [][2]interface{}{{chanID, chanBytes(layout)}})

	d, _ := decodeBytes(t, data)
	if d.ChannelLayout == nil {
		t.Fatal("expected a channel layout to be parsed")
	}
	if d.ChannelLayout.Tag != ChannelLayoutTagMPEG_5_1_A {
		t.Fatalf("expected tag %s, got %s", ChannelLayoutTagMPEG_5_1_A, d.ChannelLayout.Tag)
	}
	if got := d.ChannelLayout.ChannelCount(); got != 6 {
		t.Fatalf("expected 6 channels in the layout, got %d", got)
	}
	want := []ChannelLabel{
		ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter,
		ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect,
	}
	if got := d.ChannelLabels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected 5.1 channel order\nwant: %v\ngot:  %v", want, got)
	}
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected decoder error: %v", err)
	}
}

func TestChanChunkExplicitDescriptions(t *testing.T) {
	layout := &ChannelLayout{
		Tag: ChannelLayoutTagUseChannelDescriptions,
		Descriptions: []ChannelDescription{
			{Label: ChannelLabelCenter},
			{Label: ChannelLabelLeft},
			{Label: ChannelLabelRight, Flags: ChannelFlagsRectangularCoordinates, Coordinates: [3]float32{-1, 0, 1}},
		},
	}
	data := makeAIFF(3, 16, threeChanFrames, [][2]interface{}{{chanID, chanBytes(layout)}})

	d, buf := decodeBytes(t, data)
	if d.ChannelLayout == nil {
		t.Fatal("expected a channel layout to be parsed")
	}
	if !reflect.DeepEqual(d.ChannelLayout, layout) {
		t.Fatalf("layout mismatch\nwant: %+v\ngot:  %+v", layout, d.ChannelLayout)
	}
	want := []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight}
	if got := d.ChannelLabels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected channel order %v, got %v", want, got)
	}
	if buf.NumFrames() != len(threeChanFrames) {
		t.Fatalf("expected %d frames, got %d", len(threeChanFrames), buf.NumFrames())
	}
}

func TestChanChunkBitmap(t *testing.T) {
	bitmap := ChannelBitLeft | ChannelBitRight | ChannelBitCenter | ChannelBitLFEScreen |
		ChannelBitLeftSurroundDirect | ChannelBitRightSurroundDirect
	layout := NewChannelLayoutFromBitmap(bitmap)
	data := makeAIFF(6, 16, fiveOneFrames, [][2]interface{}{{chanID, chanBytes(layout)}})

	d, _ := decodeBytes(t, data)
	if d.ChannelLayout.Tag != ChannelLayoutTagUseChannelBitmap {
		t.Fatalf("expected the bitmap tag, got %s", d.ChannelLayout.Tag)
	}
	if d.ChannelLayout.Bitmap != bitmap {
		t.Fatalf("expected bitmap 0x%x, got 0x%x", bitmap, d.ChannelLayout.Bitmap)
	}
	want := []ChannelLabel{
		ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter,
		ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect,
	}
	if got := d.ChannelLabels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected bitmap channel order\nwant: %v\ngot:  %v", want, got)
	}
}

func TestChanChunkUnknownButValidTag(t *testing.T) {
	// An unknown but legal tag with a matching channel count must be preserved
	// and must not prevent the file from being read.
	layout := &ChannelLayout{Tag: ChannelLayoutTag((999 << 16) | 3)}
	data := makeAIFF(3, 16, threeChanFrames, [][2]interface{}{{chanID, chanBytes(layout)}})
	d, _ := decodeBytes(t, data)
	if d.ChannelLayout == nil || d.ChannelLayout.Tag != layout.Tag {
		t.Fatalf("expected the unknown tag to be preserved, got %+v", d.ChannelLayout)
	}
	if err := d.Err(); err != nil {
		t.Fatalf("an unknown tag must not be an error, got %v", err)
	}
	if labels := d.ChannelLabels(); labels != nil {
		t.Fatalf("expected no labels for an unknown tag, got %v", labels)
	}
}

func TestChanChunkUnknownLabelPreserved(t *testing.T) {
	layout := &ChannelLayout{
		Tag: ChannelLayoutTagUseChannelDescriptions,
		Descriptions: []ChannelDescription{
			{Label: ChannelLabel(77)},
			{Label: ChannelLabelLeft},
			{Label: ChannelLabelRight},
		},
	}
	data := makeAIFF(3, 16, threeChanFrames, [][2]interface{}{{chanID, chanBytes(layout)}})
	d, _ := decodeBytes(t, data)
	if got := d.ChannelLabels()[0]; got != ChannelLabel(77) {
		t.Fatalf("expected the unknown label 77 to be preserved, got %s", got)
	}
}

func TestChanChunkPreservesOtherChunks(t *testing.T) {
	// a CHAN chunk must not disturb the chunks around it.
	cate := make([]byte, 4+50*4+16)
	binary.BigEndian.PutUint32(cate[0:4], 0)
	layout := NewChannelLayoutFromTag(ChannelLayoutTagMPEG_3_0_A)
	data := makeAIFF(3, 16, threeChanFrames, [][2]interface{}{
		{chanID, chanBytes(layout)},
		{cateID, cate},
	})
	d, buf := decodeBytes(t, data)
	if buf.NumFrames() != len(threeChanFrames) {
		t.Fatalf("expected %d frames, got %d", len(threeChanFrames), buf.NumFrames())
	}
	if d.ChannelLayout == nil || d.ChannelLayout.Tag != ChannelLayoutTagMPEG_3_0_A {
		t.Fatalf("expected the MPEG_3_0_A layout, got %+v", d.ChannelLayout)
	}
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChanChunkErrors(t *testing.T) {
	valid := chanBytes(NewChannelLayoutFromTag(ChannelLayoutTagMPEG_5_1_A))
	bitmapLayout := chanBytes(NewChannelLayoutFromBitmap(ChannelBitLeft | ChannelBitRight))
	descLayout := chanBytes(&ChannelLayout{
		Tag: ChannelLayoutTagUseChannelDescriptions,
		Descriptions: []ChannelDescription{
			{Label: ChannelLabelLeft}, {Label: ChannelLabelRight},
		},
	})

	// truncateDescription declares 2 descriptions while only providing one.
	truncateDescription := append([]byte{}, descLayout[:len(descLayout)-chanDescriptionSize]...)

	tooManyDesc := make([]byte, chanHeaderSize)
	binary.BigEndian.PutUint32(tooManyDesc[8:12], maxChannelCount+1)

	tests := []struct {
		name    string
		chans   int
		payload []byte
		wantErr error
	}{
		{"too small", 6, []byte{0, 0, 0, 0}, ErrInvalidChannelLayoutSize},
		{"truncated descriptions", 6, truncateDescription, ErrTruncatedChannelLayout},
		{"too many descriptions", 6, tooManyDesc, ErrTooManyChannels},
		{"count mismatch tag", 2, valid, ErrChannelLayoutMismatch},
		{"count mismatch bitmap", 6, bitmapLayout, ErrChannelLayoutMismatch},
		{"empty bitmap", 2, chanBytes(&ChannelLayout{Tag: ChannelLayoutTagUseChannelBitmap}), ErrInvalidChannelLayout},
		{"bitmap with descriptions", 2, func() []byte {
			l := &ChannelLayout{
				Tag:          ChannelLayoutTagUseChannelBitmap,
				Bitmap:       ChannelBitLeft | ChannelBitRight,
				Descriptions: []ChannelDescription{{Label: ChannelLabelLeft}, {Label: ChannelLabelRight}},
			}
			return chanBytes(l)
		}(), ErrInvalidChannelLayout},
		{"tag with descriptions", 2, func() []byte {
			l := &ChannelLayout{
				Tag:          ChannelLayoutTagStereo,
				Descriptions: []ChannelDescription{{Label: ChannelLabelLeft}, {Label: ChannelLabelRight}},
			}
			return chanBytes(l)
		}(), ErrInvalidChannelLayout},
		{"no descriptions", 2, chanBytes(&ChannelLayout{Tag: ChannelLayoutTagUseChannelDescriptions}), ErrInvalidChannelLayout},
		{"huge channel count", 2, func() []byte {
			b := make([]byte, chanHeaderSize)
			binary.BigEndian.PutUint32(b[0:4], uint32((100<<16)|100))
			return b
		}(), ErrTooManyChannels},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frames := make([][]int, 1)
			for i := range frames {
				frames[i] = make([]int, tc.chans)
			}
			data := makeAIFF(tc.chans, 16, frames, [][2]interface{}{{chanID, tc.payload}})
			d := NewDecoder(bytes.NewReader(data))
			_, err := d.FullPCMBuffer()
			if err == nil {
				if err := d.Drain(); err == nil {
					t.Fatalf("expected an error, got none (layout %+v)", d.ChannelLayout)
				}
			}
			if !errors.Is(d.Err(), tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, d.Err())
			}
			if d.ChannelLayout != nil {
				t.Fatalf("a rejected layout must not be stored, got %+v", d.ChannelLayout)
			}
			if d.IsValidFile() {
				t.Fatal("a file with an invalid CHAN chunk must not be reported as valid")
			}
		})
	}
}

func TestChanChunkDuplicate(t *testing.T) {
	payload := chanBytes(NewChannelLayoutFromTag(ChannelLayoutTagMPEG_3_0_A))
	data := makeAIFF(3, 16, threeChanFrames, [][2]interface{}{
		{chanID, payload},
		{chanID, payload},
	})
	d := NewDecoder(bytes.NewReader(data))
	if _, err := d.FullPCMBuffer(); err == nil {
		if err := d.Drain(); err == nil {
			t.Fatal("expected a duplicate CHAN error")
		}
	}
	if !errors.Is(d.Err(), ErrDuplicateChannelLayout) {
		t.Fatalf("expected %v, got %v", ErrDuplicateChannelLayout, d.Err())
	}
}

func TestChanChunkTruncatedFile(t *testing.T) {
	// The declared chunk size promises more description bytes than the file
	// actually contains.
	payload := chanBytes(&ChannelLayout{
		Tag: ChannelLayoutTagUseChannelDescriptions,
		Descriptions: []ChannelDescription{
			{Label: ChannelLabelLeft}, {Label: ChannelLabelRight},
		},
	})
	data := makeAIFF(2, 16, [][]int{{1, 2}}, nil)
	// inject the CHAN chunk right after the COMM chunk but pretend it is
	// bigger than the data it carries.
	idx := bytes.Index(data, []byte{'S', 'S', 'N', 'D'})
	truncated := append([]byte{}, data[:idx]...)
	var hdr [8]byte
	copy(hdr[:4], chanID[:])
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload)+200))
	truncated = append(truncated, hdr[:]...)
	truncated = append(truncated, payload...)
	// fix the FORM size and append a SSND chunk so the decoder has audio data
	ssnd := data[idx:]
	truncated = append(truncated, ssnd...)
	binary.BigEndian.PutUint32(truncated[4:8], uint32(len(truncated)-8))

	d := NewDecoder(bytes.NewReader(truncated))
	_, err := d.FullPCMBuffer()
	if err == nil {
		_ = d.Drain()
	}
	if !errors.Is(d.Err(), ErrTruncatedChannelLayout) {
		t.Fatalf("expected %v, got %v", ErrTruncatedChannelLayout, d.Err())
	}
}

func TestChanChunkWithoutCOMMFirst(t *testing.T) {
	// CHAN placed before COMM must still be validated and exposed.
	layout := NewChannelLayoutFromTag(ChannelLayoutTagMPEG_3_0_A)
	payload := chanBytes(layout)
	buf := new(bytes.Buffer)
	write := func(v interface{}) { binary.Write(buf, binary.BigEndian, v) }
	body := new(bytes.Buffer)
	binary.Write(body, binary.BigEndian, chanID)
	binary.Write(body, binary.BigEndian, uint32(len(payload)))
	body.Write(payload)
	binary.Write(body, binary.BigEndian, COMMID)
	binary.Write(body, binary.BigEndian, uint32(18))
	binary.Write(body, binary.BigEndian, uint16(3))
	binary.Write(body, binary.BigEndian, uint32(1))
	binary.Write(body, binary.BigEndian, uint16(16))
	sampleRateBytes := audio.IntToIEEEFloat(44100)
	body.Write(sampleRateBytes[:])
	binary.Write(body, binary.BigEndian, SSNDID)
	binary.Write(body, binary.BigEndian, uint32(8+6))
	binary.Write(body, binary.BigEndian, uint32(0))
	binary.Write(body, binary.BigEndian, uint32(0))
	binary.Write(body, binary.BigEndian, []int16{1, 2, 3})
	write(formID)
	write(uint32(body.Len() + 4))
	write(aiffID)
	buf.Write(body.Bytes())

	d, buf2 := decodeBytes(t, buf.Bytes())
	if d.ChannelLayout == nil || d.ChannelLayout.Tag != ChannelLayoutTagMPEG_3_0_A {
		t.Fatalf("expected the CHAN layout to be exposed, got %+v", d.ChannelLayout)
	}
	if got := int(d.NumChans); got != 3 {
		t.Fatalf("expected 3 channels, got %d", got)
	}
	if buf2.NumFrames() != 1 {
		t.Fatalf("expected 1 frame, got %d", buf2.NumFrames())
	}
}

func TestChanChunkWithoutChanUnchanged(t *testing.T) {
	data := makeAIFF(3, 16, threeChanFrames, nil)
	d, buf := decodeBytes(t, data)
	if d.ChannelLayout != nil {
		t.Fatalf("expected no layout for a file without CHAN, got %+v", d.ChannelLayout)
	}
	if d.ChannelLabels() != nil {
		t.Fatalf("expected no labels for a file without CHAN, got %v", d.ChannelLabels())
	}
	if buf.NumFrames() != len(threeChanFrames) {
		t.Fatalf("expected %d frames, got %d", len(threeChanFrames), buf.NumFrames())
	}
}

func TestEncoderChanRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		layout *ChannelLayout
		frames [][]int
		chans  int
	}{
		{
			name:   "5.1 layout tag",
			layout: NewChannelLayoutFromTag(ChannelLayoutTagMPEG_5_1_A),
			frames: fiveOneFrames,
			chans:  6,
		},
		{
			name: "explicit three channel",
			layout: &ChannelLayout{
				Tag: ChannelLayoutTagUseChannelDescriptions,
				Descriptions: []ChannelDescription{
					{Label: ChannelLabelCenter},
					{Label: ChannelLabelLeft},
					{Label: ChannelLabelRight, Flags: ChannelFlagsRectangularCoordinates, Coordinates: [3]float32{-1, 0.5, 0.25}},
				},
			},
			frames: threeChanFrames,
			chans:  3,
		},
		{
			name: "bitmap 5.1",
			layout: NewChannelLayoutFromBitmap(ChannelBitLeft | ChannelBitRight | ChannelBitCenter |
				ChannelBitLFEScreen | ChannelBitLeftSurroundDirect | ChannelBitRightSurroundDirect),
			frames: fiveOneFrames,
			chans:  6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := encodeBytes(t, tc.chans, 16, tc.layout, tc.frames)
			d, buf := decodeBytes(t, data)
			if d.ChannelLayout == nil {
				t.Fatal("expected the encoded layout to be read back")
			}
			if !reflect.DeepEqual(d.ChannelLayout, tc.layout) {
				t.Fatalf("layout did not round trip\nwant: %+v\ngot:  %+v", tc.layout, d.ChannelLayout)
			}
			if got := d.ChannelLayout.ChannelCount(); got != tc.chans {
				t.Fatalf("expected %d channels, got %d", tc.chans, got)
			}
			if buf.NumFrames() != len(tc.frames) {
				t.Fatalf("expected %d frames, got %d", len(tc.frames), buf.NumFrames())
			}
			var want []int
			for _, f := range tc.frames {
				want = append(want, f...)
			}
			if !reflect.DeepEqual(buf.Data, want) {
				t.Fatalf("PCM data did not round trip\nwant: %v\ngot:  %v", want, buf.Data)
			}
		})
	}
}

func TestEncoderChanByteOrder(t *testing.T) {
	layout := &ChannelLayout{
		Tag: ChannelLayoutTagUseChannelDescriptions,
		Descriptions: []ChannelDescription{
			{Label: ChannelLabelLeft, Flags: ChannelFlagsSphericalCoordinates, Coordinates: [3]float32{30, -10, 1.5}},
		},
	}
	data := encodeBytes(t, 1, 16, layout, [][]int{{7}})

	// Locate the CHAN payload and compare against the expected big endian
	// encoding, byte for byte.
	idx := bytes.Index(data, chanID[:])
	if idx < 0 {
		t.Fatal("no CHAN chunk in the encoded file")
	}
	want := chanBytes(layout)
	got := data[idx+8 : idx+8+len(want)]
	if !bytes.Equal(got, want) {
		t.Fatalf("CHAN payload byte mismatch\nwant: % x\ngot:  % x", want, got)
	}
	// explicitly check the first coordinate is a big endian float32.
	var bits uint32
	binary.Read(bytes.NewReader(got[12+8:12+12]), binary.BigEndian, &bits)
	if v := math.Float32frombits(bits); v != 30 {
		t.Fatalf("expected 30 as the first coordinate, got %v", v)
	}
	// The declared CHAN size must be the payload size (12 + 20*n).
	if size := binary.BigEndian.Uint32(data[idx+4 : idx+8]); size != 32 {
		t.Fatalf("expected a CHAN size of 32, got %d", size)
	}
	// The FORM size must cover the whole file.
	if size := binary.BigEndian.Uint32(data[4:8]); int(size) != len(data)-8 {
		t.Fatalf("FORM size %d doesn't match file length %d", size, len(data)-8)
	}
}

func TestEncoderChanValidation(t *testing.T) {
	out := &memWriteSeeker{}
	e := NewEncoder(out, 44100, 16, 2)
	e.ChannelLayout = NewChannelLayoutFromTag(ChannelLayoutTagMPEG_5_1_A)
	buf := &audio.IntBuffer{Format: &audio.Format{NumChannels: 2, SampleRate: 44100}, Data: []int{1, 2}}
	if err := e.Write(buf); !errors.Is(err, ErrChannelLayoutMismatch) {
		t.Fatalf("expected %v, got %v", ErrChannelLayoutMismatch, err)
	}
	if out.pos != 0 {
		t.Fatalf("nothing should have been written on a validation failure, wrote %d bytes", out.pos)
	}
}

func TestEncoderChanUnknownTagRoundTrip(t *testing.T) {
	// An unknown but legal tag must survive a write/read cycle untouched.
	layout := &ChannelLayout{Tag: ChannelLayoutTag((999 << 16) | 3)}
	data := encodeBytes(t, 3, 16, layout, threeChanFrames)
	d, _ := decodeBytes(t, data)
	if !reflect.DeepEqual(d.ChannelLayout, layout) {
		t.Fatalf("unknown tag did not round trip\nwant: %+v\ngot:  %+v", layout, d.ChannelLayout)
	}
}

func TestEncoderChanOmitted(t *testing.T) {
	data := encodeBytes(t, 3, 16, nil, threeChanFrames)
	if bytes.Contains(data, chanID[:]) {
		t.Fatal("no CHAN chunk should be written when no layout is set")
	}
	d, _ := decodeBytes(t, data)
	if d.ChannelLayout != nil {
		t.Fatalf("expected no layout, got %+v", d.ChannelLayout)
	}
}

func TestChannelBitmapLabels(t *testing.T) {
	// include an undocumented high bit: it must be preserved as its ordinal
	// label instead of being dropped.
	b := ChannelBitCenter | ChannelBitLeft | ChannelBitmap(1<<19)
	if got, want := b.Count(), 3; got != want {
		t.Fatalf("bitmap count: want %d, got %d", want, got)
	}
	// labels come out in ascending label order, so center (label 3) is after
	// left (label 1), and bit 19 maps to label 20.
	want := []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabel(20)}
	if got := b.Labels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels: want %v, got %v", want, got)
	}
	if got := ChannelBitmap(0).Labels(); got != nil {
		t.Fatalf("expected nil labels for an empty bitmap, got %v", got)
	}
}

func TestChanChunkReadInfoOnly(t *testing.T) {
	// ReadInfo is the cheap path most callers use: the layout must be
	// available without touching the PCM data.
	layout := NewChannelLayoutFromTag(ChannelLayoutTagMPEG_5_1_A)
	data := makeAIFF(6, 16, fiveOneFrames, [][2]interface{}{{chanID, chanBytes(layout)}})
	d := NewDecoder(bytes.NewReader(data))
	d.ReadInfo()
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.ChannelLayout == nil || d.ChannelLayout.Tag != ChannelLayoutTagMPEG_5_1_A {
		t.Fatalf("expected the layout after ReadInfo, got %+v", d.ChannelLayout)
	}
	if d.WasPCMAccessed() {
		t.Fatal("ReadInfo shouldn't have read the PCM data")
	}
}

func TestChanFixtures(t *testing.T) {
	// ring.aif and 98_G.aif carry a stereo CHAN chunk (bitmap based).
	for _, name := range []string{"fixtures/ring.aif", "fixtures/98_G.aif"} {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		d := NewDecoder(f)
		if _, err := d.FullPCMBuffer(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d.ChannelLayout == nil {
			t.Fatalf("%s: expected a channel layout", name)
		}
		want := []ChannelLabel{ChannelLabelLeft, ChannelLabelRight}
		if got := d.ChannelLabels(); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: expected %v, got %v", name, want, got)
		}
		f.Close()
	}
}
