package aiff

import (
	"errors"
	"fmt"
	"math/bits"
)

// The CHAN chunk stores a Core Audio AudioChannelLayout structure. All of its
// fields are stored in big endian byte order:
//
//	UInt32 mChannelLayoutTag
//	UInt32 mChannelBitmap
//	UInt32 mNumberChannelDescriptions
//	AudioChannelDescription mChannelDescriptions[mNumberChannelDescriptions]
//
// AudioChannelDescription is 20 bytes:
//
//	UInt32  mChannelLabel
//	UInt32  mChannelFlags
//	Float32 mCoordinates[3]
//
// See https://developer.apple.com/documentation/quicktime-file-format/audio_channel_layout_atom
// and the CAF specification (CAFChannelLayout).

const (
	// maxChannelCount is the highest number of channels accepted in a CHAN
	// chunk. Real world AIFF files stay well below this limit; the cap keeps a
	// malformed chunk from forcing a huge allocation.
	maxChannelCount = 64

	// chanHeaderSize is the size of the fixed part of an AudioChannelLayout.
	chanHeaderSize = 12
	// chanDescriptionSize is the size of an AudioChannelDescription.
	chanDescriptionSize = 20
)

var (
	// ErrInvalidChannelLayout reports a structurally invalid CHAN chunk, such
	// as a bitmap tag without a bitmap or a layout tag mixed with channel
	// descriptions.
	ErrInvalidChannelLayout = errors.New("invalid channel layout")
	// ErrInvalidChannelLayoutSize reports a CHAN chunk too small to contain an
	// AudioChannelLayout header.
	ErrInvalidChannelLayoutSize = errors.New("invalid CHAN chunk size")
	// ErrTruncatedChannelLayout reports a CHAN chunk whose size doesn't cover
	// the declared number of channel descriptions.
	ErrTruncatedChannelLayout = errors.New("truncated CHAN channel descriptions")
	// ErrDuplicateChannelLayout reports a second CHAN chunk in the same file.
	ErrDuplicateChannelLayout = errors.New("duplicate CHAN chunk")
	// ErrChannelLayoutMismatch reports a CHAN layout whose channel count
	// doesn't match the channel count declared by the COMM chunk.
	ErrChannelLayoutMismatch = errors.New("channel layout does not match COMM channel count")
	// ErrTooManyChannels reports a channel count above maxChannelCount.
	ErrTooManyChannels = errors.New("too many channels in channel layout")
)

// ChannelLabel is a Core Audio channel label (the mChannelLabel field of an
// AudioChannelDescription).
type ChannelLabel uint32

// Core Audio channel labels. Labels 1-18 also correspond to the bits of a
// channel bitmap (bit N is label N+1).
const (
	ChannelLabelLeft                 ChannelLabel = 1
	ChannelLabelRight                ChannelLabel = 2
	ChannelLabelCenter               ChannelLabel = 3
	ChannelLabelLFEScreen            ChannelLabel = 4
	ChannelLabelLeftSurround         ChannelLabel = 5
	ChannelLabelRightSurround        ChannelLabel = 6
	ChannelLabelLeftCenter           ChannelLabel = 7
	ChannelLabelRightCenter          ChannelLabel = 8
	ChannelLabelCenterSurround       ChannelLabel = 9
	ChannelLabelLeftSurroundDirect   ChannelLabel = 10
	ChannelLabelRightSurroundDirect  ChannelLabel = 11
	ChannelLabelTopCenterSurround    ChannelLabel = 12
	ChannelLabelVerticalHeightLeft   ChannelLabel = 13
	ChannelLabelVerticalHeightCenter ChannelLabel = 14
	ChannelLabelVerticalHeightRight  ChannelLabel = 15
	ChannelLabelTopBackLeft          ChannelLabel = 16
	ChannelLabelTopBackCenter        ChannelLabel = 17
	ChannelLabelTopBackRight         ChannelLabel = 18
	ChannelLabelLeftWide             ChannelLabel = 35
	ChannelLabelRightWide            ChannelLabel = 36
	ChannelLabelLFE2                 ChannelLabel = 37
	ChannelLabelLeftTotal            ChannelLabel = 38
	ChannelLabelRightTotal           ChannelLabel = 39
	ChannelLabelHearingImpaired      ChannelLabel = 40
	ChannelLabelNarration            ChannelLabel = 41
	ChannelLabelMono                 ChannelLabel = 42
	ChannelLabelDialogCentricMix     ChannelLabel = 43
	ChannelLabelCenterSurroundDirect ChannelLabel = 44
	ChannelLabelHaptic               ChannelLabel = 45
	ChannelLabelUseCoordinates       ChannelLabel = 100
	ChannelLabelUnknown              ChannelLabel = 0xFFFFFFFF
)

var channelLabelNames = map[ChannelLabel]string{
	ChannelLabelLeft:                 "Left",
	ChannelLabelRight:                "Right",
	ChannelLabelCenter:               "Center",
	ChannelLabelLFEScreen:            "LFE",
	ChannelLabelLeftSurround:         "LeftSurround",
	ChannelLabelRightSurround:        "RightSurround",
	ChannelLabelLeftCenter:           "LeftCenter",
	ChannelLabelRightCenter:          "RightCenter",
	ChannelLabelCenterSurround:       "CenterSurround",
	ChannelLabelLeftSurroundDirect:   "LeftSurroundDirect",
	ChannelLabelRightSurroundDirect:  "RightSurroundDirect",
	ChannelLabelTopCenterSurround:    "TopCenterSurround",
	ChannelLabelVerticalHeightLeft:   "VerticalHeightLeft",
	ChannelLabelVerticalHeightCenter: "VerticalHeightCenter",
	ChannelLabelVerticalHeightRight:  "VerticalHeightRight",
	ChannelLabelTopBackLeft:          "TopBackLeft",
	ChannelLabelTopBackCenter:        "TopBackCenter",
	ChannelLabelTopBackRight:         "TopBackRight",
	ChannelLabelLeftWide:             "LeftWide",
	ChannelLabelRightWide:            "RightWide",
	ChannelLabelLFE2:                 "LFE2",
	ChannelLabelLeftTotal:            "LeftTotal",
	ChannelLabelRightTotal:           "RightTotal",
	ChannelLabelHearingImpaired:      "HearingImpaired",
	ChannelLabelNarration:            "Narration",
	ChannelLabelMono:                 "Mono",
	ChannelLabelDialogCentricMix:     "DialogCentricMix",
	ChannelLabelCenterSurroundDirect: "CenterSurroundDirect",
	ChannelLabelHaptic:               "Haptic",
	ChannelLabelUseCoordinates:       "UseCoordinates",
	ChannelLabelUnknown:              "Unknown",
}

// String returns the Core Audio name of the label when known.
func (l ChannelLabel) String() string {
	if name, ok := channelLabelNames[l]; ok {
		return name
	}
	return fmt.Sprintf("ChannelLabel(%d)", uint32(l))
}

// ChannelBitmap is a Core Audio channel bitmap (mChannelBitmap). Bit N set
// means channel label N+1 is present.
type ChannelBitmap uint32

// Core Audio channel bitmap bits.
const (
	ChannelBitLeft                 ChannelBitmap = 1 << 0
	ChannelBitRight                ChannelBitmap = 1 << 1
	ChannelBitCenter               ChannelBitmap = 1 << 2
	ChannelBitLFEScreen            ChannelBitmap = 1 << 3
	ChannelBitLeftSurround         ChannelBitmap = 1 << 4
	ChannelBitRightSurround        ChannelBitmap = 1 << 5
	ChannelBitLeftCenter           ChannelBitmap = 1 << 6
	ChannelBitRightCenter          ChannelBitmap = 1 << 7
	ChannelBitCenterSurround       ChannelBitmap = 1 << 8
	ChannelBitLeftSurroundDirect   ChannelBitmap = 1 << 9
	ChannelBitRightSurroundDirect  ChannelBitmap = 1 << 10
	ChannelBitTopCenterSurround    ChannelBitmap = 1 << 11
	ChannelBitVerticalHeightLeft   ChannelBitmap = 1 << 12
	ChannelBitVerticalHeightCenter ChannelBitmap = 1 << 13
	ChannelBitVerticalHeightRight  ChannelBitmap = 1 << 14
	ChannelBitTopBackLeft          ChannelBitmap = 1 << 15
	ChannelBitTopBackCenter        ChannelBitmap = 1 << 16
	ChannelBitTopBackRight         ChannelBitmap = 1 << 17
)

// Count returns the number of channels set in the bitmap.
func (b ChannelBitmap) Count() int {
	return bits.OnesCount32(uint32(b))
}

// Labels returns the labels set in the bitmap, in ascending label order. Bits
// above the documented range are preserved in the bitmap and returned as the
// corresponding ordinal labels.
func (b ChannelBitmap) Labels() []ChannelLabel {
	if b == 0 {
		return nil
	}
	labels := make([]ChannelLabel, 0, b.Count())
	for i := 0; i < 32; i++ {
		if uint32(b)&(1<<uint(i)) != 0 {
			labels = append(labels, ChannelLabel(i+1))
		}
	}
	return labels
}

// ChannelDescription is a Core Audio AudioChannelDescription.
type ChannelDescription struct {
	Label ChannelLabel
	// Flags selects how Coordinates are interpreted.
	Flags uint32
	// Coordinates holds x, y, z when rectangular coordinates are used, or
	// azimuth, elevation, distance when spherical coordinates are used.
	Coordinates [3]float32
}

// Channel description coordinate flags.
const (
	ChannelFlagsRectangularCoordinates uint32 = 1 << 0
	ChannelFlagsSphericalCoordinates   uint32 = 1 << 1
	ChannelFlagsMeters                 uint32 = 1 << 2
)

// ChannelLayoutTag is a Core Audio channel layout tag (mChannelLayoutTag). The
// low 16 bits hold the number of channels for regular tags.
type ChannelLayoutTag uint32

// Special channel layout tags.
const (
	// ChannelLayoutTagUseChannelDescriptions means the layout is described by
	// the explicit channel descriptions.
	ChannelLayoutTagUseChannelDescriptions ChannelLayoutTag = 0 << 16
	// ChannelLayoutTagUseChannelBitmap means the layout is described by the
	// channel bitmap.
	ChannelLayoutTagUseChannelBitmap ChannelLayoutTag = 1 << 16
)

// Core Audio channel layout tags. The values follow the layout tag encoding
// (tag<<16)|channelCount documented in CoreAudioTypes.h.
const (
	ChannelLayoutTagMono               ChannelLayoutTag = (100 << 16) | 1
	ChannelLayoutTagStereo             ChannelLayoutTag = (101 << 16) | 2
	ChannelLayoutTagStereoHeadphones   ChannelLayoutTag = (102 << 16) | 2
	ChannelLayoutTagMatrixStereo       ChannelLayoutTag = (103 << 16) | 2
	ChannelLayoutTagMidiSide           ChannelLayoutTag = (104 << 16) | 2
	ChannelLayoutTagXY                 ChannelLayoutTag = (105 << 16) | 2
	ChannelLayoutTagBinaural           ChannelLayoutTag = (106 << 16) | 2
	ChannelLayoutTagQuadraphonic       ChannelLayoutTag = (108 << 16) | 4
	ChannelLayoutTagPentagonal         ChannelLayoutTag = (109 << 16) | 5
	ChannelLayoutTagHexagonal          ChannelLayoutTag = (110 << 16) | 6
	ChannelLayoutTagOctagonal          ChannelLayoutTag = (111 << 16) | 8
	ChannelLayoutTagCube               ChannelLayoutTag = (112 << 16) | 8
	ChannelLayoutTagMPEG_3_0_A         ChannelLayoutTag = (113 << 16) | 3
	ChannelLayoutTagMPEG_3_0_B         ChannelLayoutTag = (114 << 16) | 3
	ChannelLayoutTagMPEG_4_0_A         ChannelLayoutTag = (115 << 16) | 4
	ChannelLayoutTagMPEG_4_0_B         ChannelLayoutTag = (116 << 16) | 4
	ChannelLayoutTagMPEG_5_0_A         ChannelLayoutTag = (117 << 16) | 5
	ChannelLayoutTagMPEG_5_0_B         ChannelLayoutTag = (118 << 16) | 5
	ChannelLayoutTagMPEG_5_0_C         ChannelLayoutTag = (119 << 16) | 5
	ChannelLayoutTagMPEG_5_0_D         ChannelLayoutTag = (120 << 16) | 5
	ChannelLayoutTagMPEG_5_1_A         ChannelLayoutTag = (121 << 16) | 6
	ChannelLayoutTagMPEG_5_1_B         ChannelLayoutTag = (122 << 16) | 6
	ChannelLayoutTagMPEG_5_1_C         ChannelLayoutTag = (123 << 16) | 6
	ChannelLayoutTagMPEG_5_1_D         ChannelLayoutTag = (124 << 16) | 6
	ChannelLayoutTagMPEG_6_1_A         ChannelLayoutTag = (125 << 16) | 7
	ChannelLayoutTagMPEG_7_1_A         ChannelLayoutTag = (126 << 16) | 8
	ChannelLayoutTagMPEG_7_1_B         ChannelLayoutTag = (127 << 16) | 8
	ChannelLayoutTagMPEG_7_1_C         ChannelLayoutTag = (128 << 16) | 8
	ChannelLayoutTagEmagicDefault_7_1  ChannelLayoutTag = (129 << 16) | 8
	ChannelLayoutTagSMPTE_DTV          ChannelLayoutTag = (130 << 16) | 8
	ChannelLayoutTagITU_2_1            ChannelLayoutTag = (131 << 16) | 3
	ChannelLayoutTagITU_2_2            ChannelLayoutTag = (132 << 16) | 4
	ChannelLayoutTagDVD_4              ChannelLayoutTag = (133 << 16) | 3
	ChannelLayoutTagDVD_5              ChannelLayoutTag = (134 << 16) | 4
	ChannelLayoutTagDVD_6              ChannelLayoutTag = (135 << 16) | 5
	ChannelLayoutTagDVD_10             ChannelLayoutTag = (136 << 16) | 4
	ChannelLayoutTagDVD_11             ChannelLayoutTag = (137 << 16) | 5
	ChannelLayoutTagDVD_18             ChannelLayoutTag = (138 << 16) | 5
	ChannelLayoutTagAudioUnit_6_0      ChannelLayoutTag = (139 << 16) | 6
	ChannelLayoutTagAudioUnit_7_0      ChannelLayoutTag = (140 << 16) | 7
	ChannelLayoutTagAAC_6_0            ChannelLayoutTag = (141 << 16) | 6
	ChannelLayoutTagAAC_6_1            ChannelLayoutTag = (142 << 16) | 7
	ChannelLayoutTagAAC_7_0            ChannelLayoutTag = (143 << 16) | 7
	ChannelLayoutTagAAC_Octagonal      ChannelLayoutTag = (144 << 16) | 8
	ChannelLayoutTagAudioUnit_7_0Front ChannelLayoutTag = (148 << 16) | 7
	ChannelLayoutTagAC3_1_0_1          ChannelLayoutTag = (149 << 16) | 2
	ChannelLayoutTagAC3_3_0            ChannelLayoutTag = (150 << 16) | 3
	ChannelLayoutTagAC3_3_1            ChannelLayoutTag = (151 << 16) | 4
	ChannelLayoutTagAC3_3_0_1          ChannelLayoutTag = (152 << 16) | 4
	ChannelLayoutTagAC3_2_1_1          ChannelLayoutTag = (153 << 16) | 4
	ChannelLayoutTagAC3_3_1_1          ChannelLayoutTag = (154 << 16) | 5
	ChannelLayoutTagEAC3_6_0_A         ChannelLayoutTag = (155 << 16) | 6
	ChannelLayoutTagEAC3_7_0_A         ChannelLayoutTag = (156 << 16) | 7
	ChannelLayoutTagEAC3_6_1_A         ChannelLayoutTag = (157 << 16) | 7
	ChannelLayoutTagEAC3_6_1_B         ChannelLayoutTag = (158 << 16) | 7
	ChannelLayoutTagEAC3_6_1_C         ChannelLayoutTag = (159 << 16) | 7
	ChannelLayoutTagEAC3_7_1_A         ChannelLayoutTag = (160 << 16) | 8
	ChannelLayoutTagEAC3_7_1_B         ChannelLayoutTag = (161 << 16) | 8
	ChannelLayoutTagEAC3_7_1_C         ChannelLayoutTag = (162 << 16) | 8
	ChannelLayoutTagEAC3_7_1_D         ChannelLayoutTag = (163 << 16) | 8
	ChannelLayoutTagEAC3_7_1_E         ChannelLayoutTag = (164 << 16) | 8
	ChannelLayoutTagEAC3_7_1_F         ChannelLayoutTag = (165 << 16) | 8
	ChannelLayoutTagEAC3_7_1_G         ChannelLayoutTag = (166 << 16) | 8
	ChannelLayoutTagEAC3_7_1_H         ChannelLayoutTag = (167 << 16) | 8
	ChannelLayoutTagDTS_3_1            ChannelLayoutTag = (168 << 16) | 4
	ChannelLayoutTagDTS_4_1            ChannelLayoutTag = (169 << 16) | 5
	ChannelLayoutTagDTS_6_0_A          ChannelLayoutTag = (170 << 16) | 6
	ChannelLayoutTagDTS_6_0_B          ChannelLayoutTag = (171 << 16) | 6
	ChannelLayoutTagDTS_6_0_C          ChannelLayoutTag = (172 << 16) | 6
	ChannelLayoutTagDTS_6_1_A          ChannelLayoutTag = (173 << 16) | 7
	ChannelLayoutTagDTS_6_1_B          ChannelLayoutTag = (174 << 16) | 7
	ChannelLayoutTagDTS_6_1_C          ChannelLayoutTag = (175 << 16) | 7
	ChannelLayoutTagDTS_6_1_D          ChannelLayoutTag = (182 << 16) | 7
	ChannelLayoutTagDTS_7_0            ChannelLayoutTag = (176 << 16) | 7
	ChannelLayoutTagDTS_7_1            ChannelLayoutTag = (177 << 16) | 8
	ChannelLayoutTagDTS_8_0_A          ChannelLayoutTag = (178 << 16) | 8
	ChannelLayoutTagDTS_8_0_B          ChannelLayoutTag = (179 << 16) | 8
	ChannelLayoutTagDTS_8_1_A          ChannelLayoutTag = (180 << 16) | 9
	ChannelLayoutTagDTS_8_1_B          ChannelLayoutTag = (181 << 16) | 9
)

type channelLayoutTagInfo struct {
	name   string
	labels []ChannelLabel
}

var channelLayoutTagTable = map[ChannelLayoutTag]channelLayoutTagInfo{
	ChannelLayoutTagMono:               {"Mono", []ChannelLabel{ChannelLabelCenter}},
	ChannelLayoutTagStereo:             {"Stereo", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight}},
	ChannelLayoutTagStereoHeadphones:   {"StereoHeadphones", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight}},
	ChannelLayoutTagMatrixStereo:       {"MatrixStereo", []ChannelLabel{ChannelLabelLeftTotal, ChannelLabelRightTotal}},
	ChannelLayoutTagMidiSide:           {"MidiSide", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight}},
	ChannelLayoutTagXY:                 {"XY", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight}},
	ChannelLayoutTagBinaural:           {"Binaural", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight}},
	ChannelLayoutTagQuadraphonic:       {"Quadraphonic", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagPentagonal:         {"Pentagonal", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelCenter}},
	ChannelLayoutTagHexagonal:          {"Hexagonal", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelCenter, ChannelLabelCenterSurround}},
	ChannelLayoutTagOctagonal:          {"Octagonal", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelCenter, ChannelLabelCenterSurround, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagCube:               {"Cube", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelVerticalHeightLeft, ChannelLabelVerticalHeightRight, ChannelLabelTopBackLeft, ChannelLabelTopBackRight}},
	ChannelLayoutTagMPEG_3_0_A:         {"MPEG_3_0_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter}},
	ChannelLayoutTagMPEG_3_0_B:         {"MPEG_3_0_B", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight}},
	ChannelLayoutTagMPEG_4_0_A:         {"MPEG_4_0_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelCenterSurround}},
	ChannelLayoutTagMPEG_4_0_B:         {"MPEG_4_0_B", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenterSurround}},
	ChannelLayoutTagMPEG_5_0_A:         {"MPEG_5_0_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagMPEG_5_0_B:         {"MPEG_5_0_B", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenter}},
	ChannelLayoutTagMPEG_5_0_C:         {"MPEG_5_0_C", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagMPEG_5_0_D:         {"MPEG_5_0_D", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagMPEG_5_1_A:         {"MPEG_5_1_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagMPEG_5_1_B:         {"MPEG_5_1_B", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenter, ChannelLabelLFEScreen}},
	ChannelLayoutTagMPEG_5_1_C:         {"MPEG_5_1_C", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
	ChannelLayoutTagMPEG_5_1_D:         {"MPEG_5_1_D", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
	ChannelLayoutTagMPEG_6_1_A:         {"MPEG_6_1_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenterSurround}},
	ChannelLayoutTagMPEG_7_1_A:         {"MPEG_7_1_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftCenter, ChannelLabelRightCenter}},
	ChannelLayoutTagMPEG_7_1_B:         {"MPEG_7_1_B", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeftCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
	ChannelLayoutTagMPEG_7_1_C:         {"MPEG_7_1_C", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagEmagicDefault_7_1:  {"EmagicDefault_7_1", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelLeftCenter, ChannelLabelRightCenter}},
	ChannelLayoutTagSMPTE_DTV:          {"SMPTE_DTV", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftTotal, ChannelLabelRightTotal}},
	ChannelLayoutTagITU_2_1:            {"ITU_2_1", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenterSurround}},
	ChannelLayoutTagITU_2_2:            {"ITU_2_2", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagDVD_4:              {"DVD_4", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLFEScreen}},
	ChannelLayoutTagDVD_5:              {"DVD_5", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLFEScreen, ChannelLabelCenterSurround}},
	ChannelLayoutTagDVD_6:              {"DVD_6", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLFEScreen, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagDVD_10:             {"DVD_10", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen}},
	ChannelLayoutTagDVD_11:             {"DVD_11", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenter, ChannelLabelLFEScreen, ChannelLabelCenterSurround}},
	ChannelLayoutTagDVD_18:             {"DVD_18", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
	ChannelLayoutTagAudioUnit_6_0:      {"AudioUnit_6_0", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenter, ChannelLabelCenterSurround}},
	ChannelLayoutTagAudioUnit_7_0:      {"AudioUnit_7_0", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenter, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagAudioUnit_7_0Front: {"AudioUnit_7_0_Front", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenter, ChannelLabelLeftCenter, ChannelLabelRightCenter}},
	ChannelLayoutTagAAC_6_0:            {"AAC_6_0", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenterSurround}},
	ChannelLayoutTagAAC_6_1:            {"AAC_6_1", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenterSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagAAC_7_0:            {"AAC_7_0", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagAAC_Octagonal:      {"AAC_Octagonal", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelCenterSurround}},
	ChannelLayoutTagAC3_1_0_1:          {"AC3_1_0_1", []ChannelLabel{ChannelLabelCenter, ChannelLabelLFEScreen}},
	ChannelLayoutTagAC3_3_0:            {"AC3_3_0", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight}},
	ChannelLayoutTagAC3_3_1:            {"AC3_3_1", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelCenterSurround}},
	ChannelLayoutTagAC3_3_0_1:          {"AC3_3_0_1", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLFEScreen}},
	ChannelLayoutTagAC3_2_1_1:          {"AC3_2_1_1", []ChannelLabel{ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenterSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagAC3_3_1_1:          {"AC3_3_1_1", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelCenterSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagEAC3_6_0_A:         {"EAC3_6_0_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelCenterSurround}},
	ChannelLayoutTagEAC3_7_0_A:         {"EAC3_7_0_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagEAC3_6_1_A:         {"EAC3_6_1_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelCenterSurround}},
	ChannelLayoutTagEAC3_6_1_B:         {"EAC3_6_1_B", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelTopCenterSurround}},
	ChannelLayoutTagEAC3_6_1_C:         {"EAC3_6_1_C", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelVerticalHeightCenter}},
	ChannelLayoutTagEAC3_7_1_A:         {"EAC3_7_1_A", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagEAC3_7_1_B:         {"EAC3_7_1_B", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelLeftCenter, ChannelLabelRightCenter}},
	ChannelLayoutTagEAC3_7_1_D:         {"EAC3_7_1_D", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelLeftWide, ChannelLabelRightWide}},
	ChannelLayoutTagEAC3_7_1_E:         {"EAC3_7_1_E", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelVerticalHeightLeft, ChannelLabelVerticalHeightRight}},
	ChannelLayoutTagEAC3_7_1_F:         {"EAC3_7_1_F", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelCenterSurround, ChannelLabelTopCenterSurround}},
	ChannelLayoutTagEAC3_7_1_G:         {"EAC3_7_1_G", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelCenterSurround, ChannelLabelVerticalHeightCenter}},
	ChannelLayoutTagEAC3_7_1_H:         {"EAC3_7_1_H", []ChannelLabel{ChannelLabelLeft, ChannelLabelCenter, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelTopCenterSurround, ChannelLabelVerticalHeightCenter}},
	ChannelLayoutTagDTS_3_1:            {"DTS_3_1", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_4_1:            {"DTS_4_1", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelCenterSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_6_0_A:          {"DTS_6_0_A", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagDTS_6_0_B:          {"DTS_6_0_B", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelTopCenterSurround}},
	ChannelLayoutTagDTS_6_0_C:          {"DTS_6_0_C", []ChannelLabel{ChannelLabelCenter, ChannelLabelCenterSurround, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagDTS_6_1_A:          {"DTS_6_1_A", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_6_1_B:          {"DTS_6_1_B", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelTopCenterSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_6_1_C:          {"DTS_6_1_C", []ChannelLabel{ChannelLabelCenter, ChannelLabelCenterSurround, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_6_1_D:          {"DTS_6_1_D", []ChannelLabel{ChannelLabelCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen, ChannelLabelCenterSurround}},
	ChannelLayoutTagDTS_7_0:            {"DTS_7_0", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagDTS_7_1:            {"DTS_7_1", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_8_0_A:          {"DTS_8_0_A", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftSurround, ChannelLabelRightSurround}},
	ChannelLayoutTagDTS_8_0_B:          {"DTS_8_0_B", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelCenterSurround, ChannelLabelRightSurroundDirect}},
	ChannelLayoutTagDTS_8_1_A:          {"DTS_8_1_A", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelRightSurroundDirect, ChannelLabelLeftSurround, ChannelLabelRightSurround, ChannelLabelLFEScreen}},
	ChannelLayoutTagDTS_8_1_B:          {"DTS_8_1_B", []ChannelLabel{ChannelLabelLeftCenter, ChannelLabelCenter, ChannelLabelRightCenter, ChannelLabelLeft, ChannelLabelRight, ChannelLabelLeftSurroundDirect, ChannelLabelCenterSurround, ChannelLabelRightSurroundDirect, ChannelLabelLFEScreen}},
}

// String returns the Core Audio name of the layout tag when known.
func (t ChannelLayoutTag) String() string {
	if info, ok := channelLayoutTagTable[t]; ok {
		return info.name
	}
	switch t {
	case ChannelLayoutTagUseChannelDescriptions:
		return "UseChannelDescriptions"
	case ChannelLayoutTagUseChannelBitmap:
		return "UseChannelBitmap"
	}
	return fmt.Sprintf("ChannelLayoutTag(0x%08x)", uint32(t))
}

// ChannelCount returns the channel count encoded in the layout tag.
func (t ChannelLayoutTag) ChannelCount() int {
	return int(t & 0xFFFF)
}

// ChannelLayout is the content of a CHAN chunk. Tag and Bitmap are preserved
// verbatim so unknown but legal layout tags round trip. Descriptions holds the
// explicit channel descriptions when Tag is
// ChannelLayoutTagUseChannelDescriptions.
type ChannelLayout struct {
	Tag          ChannelLayoutTag
	Bitmap       ChannelBitmap
	Descriptions []ChannelDescription
}

// NewChannelLayoutFromTag returns a layout for a predefined layout tag.
func NewChannelLayoutFromTag(tag ChannelLayoutTag) *ChannelLayout {
	return &ChannelLayout{Tag: tag}
}

// NewChannelLayoutFromBitmap returns a layout described by a channel bitmap.
func NewChannelLayoutFromBitmap(bitmap ChannelBitmap) *ChannelLayout {
	return &ChannelLayout{Tag: ChannelLayoutTagUseChannelBitmap, Bitmap: bitmap}
}

// NewChannelLayoutFromLabels returns a layout with one explicit channel
// description per label, preserving the given channel order.
func NewChannelLayoutFromLabels(labels ...ChannelLabel) *ChannelLayout {
	descs := make([]ChannelDescription, len(labels))
	for i, label := range labels {
		descs[i].Label = label
	}
	return &ChannelLayout{Tag: ChannelLayoutTagUseChannelDescriptions, Descriptions: descs}
}

// ChannelCount returns the number of channels declared by the layout. It
// returns 0 for an unknown tag that doesn't encode a channel count.
func (l *ChannelLayout) ChannelCount() int {
	if l == nil {
		return 0
	}
	switch l.Tag {
	case ChannelLayoutTagUseChannelDescriptions:
		return len(l.Descriptions)
	case ChannelLayoutTagUseChannelBitmap:
		return l.Bitmap.Count()
	default:
		return l.Tag.ChannelCount()
	}
}

// Labels returns the channel labels in the order the channels appear in the
// audio data. It returns nil when the layout tag is unknown and no explicit
// descriptions are available.
func (l *ChannelLayout) Labels() []ChannelLabel {
	if l == nil {
		return nil
	}
	switch l.Tag {
	case ChannelLayoutTagUseChannelDescriptions:
		if len(l.Descriptions) == 0 {
			return nil
		}
		labels := make([]ChannelLabel, len(l.Descriptions))
		for i, desc := range l.Descriptions {
			labels[i] = desc.Label
		}
		return labels
	case ChannelLayoutTagUseChannelBitmap:
		return l.Bitmap.Labels()
	default:
		info, ok := channelLayoutTagTable[l.Tag]
		if !ok || len(info.labels) == 0 {
			return nil
		}
		labels := make([]ChannelLabel, len(info.labels))
		copy(labels, info.labels)
		return labels
	}
}

// validate checks that the layout is structurally sound and that the number of
// channels it describes matches numChans (the COMM channel count).
func (l *ChannelLayout) validate(numChans int) error {
	if l == nil {
		return nil
	}
	if numChans < 1 {
		return fmt.Errorf("%w: COMM declares %d channels", ErrInvalidChannelLayout, numChans)
	}
	if numChans > maxChannelCount {
		return fmt.Errorf("%w: COMM declares %d channels", ErrTooManyChannels, numChans)
	}
	if n := l.Tag.ChannelCount(); n > maxChannelCount {
		return fmt.Errorf("%w: layout tag declares %d channels", ErrTooManyChannels, n)
	}

	switch l.Tag {
	case ChannelLayoutTagUseChannelDescriptions:
		if len(l.Descriptions) == 0 {
			return fmt.Errorf("%w: no channel descriptions", ErrInvalidChannelLayout)
		}
		if len(l.Descriptions) > maxChannelCount {
			return fmt.Errorf("%w: %d channel descriptions", ErrTooManyChannels, len(l.Descriptions))
		}
		if len(l.Descriptions) != numChans {
			return fmt.Errorf("%w: %d channel descriptions for %d channels", ErrChannelLayoutMismatch, len(l.Descriptions), numChans)
		}
	case ChannelLayoutTagUseChannelBitmap:
		if l.Bitmap == 0 {
			return fmt.Errorf("%w: empty channel bitmap", ErrInvalidChannelLayout)
		}
		if len(l.Descriptions) != 0 {
			return fmt.Errorf("%w: bitmap layout with %d channel descriptions", ErrInvalidChannelLayout, len(l.Descriptions))
		}
		if n := l.Bitmap.Count(); n > maxChannelCount {
			return fmt.Errorf("%w: bitmap declares %d channels", ErrTooManyChannels, n)
		}
		if n := l.Bitmap.Count(); n != numChans {
			return fmt.Errorf("%w: bitmap has %d channels, COMM has %d", ErrChannelLayoutMismatch, n, numChans)
		}
	default:
		if len(l.Descriptions) != 0 {
			return fmt.Errorf("%w: layout tag %s with %d channel descriptions", ErrInvalidChannelLayout, l.Tag, len(l.Descriptions))
		}
		if n := l.Tag.ChannelCount(); n != numChans {
			return fmt.Errorf("%w: tag declares %d channels, COMM has %d", ErrChannelLayoutMismatch, n, numChans)
		}
	}
	return nil
}
