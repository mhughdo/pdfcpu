/*
Copyright 2018 The pdfcpu Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pdfcpu

import "fmt"

// DisplayUnit is the metric unit used to output paper sizes.
type DisplayUnit int

// The available display units.
const (
	POINTS DisplayUnit = iota
	INCHES
	CENTIMETRES
	MILLIMETRES
)

// Dim represents the dimensions of a rectangular view medium
// like a PDF page, a sheet of paper or an image grid.
type Dim struct {
	w, h float64
}

// ToInches converts d to inches.
func (d Dim) ToInches() Dim {
	return Dim{d.w / 72, d.h / 72}
}

// ToCentimetres converts d to centimetres.
func (d Dim) ToCentimetres() Dim {
	return Dim{d.w / 72 * 2.54, d.h / 72 * 2.54}
}

// ToMillimetres converts d to centimetres.
func (d Dim) ToMillimetres() Dim {
	return Dim{d.w / 72 * 25.4, d.h / 72 * 25.4}
}

// AspectRatio returns the relation between width and height.
func (d Dim) AspectRatio() float64 {
	return d.w / d.h
}

// Landscape returns true if d is in landscape mode.
func (d Dim) Landscape() bool {
	return d.AspectRatio() > 1
}

// Portrait returns true if d is in portrait mode.
func (d Dim) Portrait() bool {
	return d.AspectRatio() < 1
}

func (d Dim) String() string {
	return fmt.Sprintf("%fx%f points", d.w, d.h)
}

// PaperSize is a map of known paper sizes in user units (=72 dpi pixels).
var PaperSize = map[string]*Dim{

	// ISO 216:1975 A
	"4A0": {4768, 6741}, // 66 1/4" x 93 5/8"	1682 x 2378 mm
	"2A0": {3370, 4768}, // 46 3/4" x 66 1/4"	1189 x 1682 mm
	"A0":  {2384, 3370}, //     33" x 46 3/4"	 841 x 1189 mm
	"A1":  {1684, 2384}, // 23 3/8" x 33"		 594 x 841 mm
	"A2":  {1191, 1684}, // 16 1/2" x 23 3/8"	 420 x 594 mm
	"A3":  {842, 1191},  // 11 3/4" x 16 1/2"	 297 x 420 mm
	"A4":  {595, 842},   //  8 1/4" x 11 3/4"	 210 x 297 mm
	"A5":  {420, 595},   //  5 7/8" x 8 1/4"	 148 x 210 mm
	"A6":  {298, 420},   //  4 1/8" x 5 7/8"	 105 x 148 mm
	"A7":  {210, 298},   //  2 7/8" x 4 1/8"	  74 x 105 mm
	"A8":  {147, 210},   //      2" x 2 7/8"	  52 x 74 mm
	"A9":  {105, 147},   //  1 1/2" x 2"		  37 x 52 mm
	"A10": {74, 105},    //      1" x 1 1/2"	  26 x 37 mm

	// ISO 216:1975 B
	"B0+": {3170, 4479}, //     44" x 62 1/4"	1118 x 1580 mm
	"B0":  {2835, 4008}, // 39 3/8" x 55 3/4"	1000 x 1414 mm
	"B1+": {2041, 2892}, // 28 3/8" x 40 1/8"	 720 x 1020 mm
	"B1":  {2004, 2835}, // 27 3/4" x 39 3/8"	 707 x 1000 mm
	"B2+": {1474, 2041}, // 20 1/2" x 28 3/8"	 520 x 720 mm
	"B2":  {1417, 2004}, // 19 3/4" x 27 3/4"	 500 x 707 mm
	"B3":  {1001, 1417}, // 13 7/8" x 19 3/4"	 353 x 500 mm
	"B4":  {709, 1001},  //  9 7/8" x 13 7/8"	 250 x 353 mm
	"B5":  {499, 709},   //      7" x 9 7/8"	 176 x 250 mm
	"B6":  {354, 499},   //  4 7/8" x 7"		 125 x 176 mm
	"B7":  {249, 354},   //  3 1/2" x 4 7/8"	  88 x 125 mm
	"B8":  {176, 249},   //  2 1/2" x 3 1/2"	  62 x 88 mm
	"B9":  {125, 176},   //  1 3/4" x 2 1/2"	  44 x 62 mm
	"B10": {88, 125},    //  1 1/4" x 1 3/4"	  31 x 44 mm

	// ISO 269:1985 envelopes aka ISO C
	"C0":  {2599, 3677}, //     36" x 51"		917 x 1297 mm
	"C1":  {1837, 2599}, // 25 1/2" x 36"		648 x 917 mm
	"C2":  {1298, 1837}, //     18" x 25 1/2"	458 x 648 mm
	"C3":  {918, 1298},  // 12 3/4" x 18"		324 x 458 mm
	"C4":  {649, 918},   //      9" x 12 3/4"	229 x 324 mm
	"C5":  {459, 649},   //  6 3/8" x 9"		162 x 229 mm
	"C6":  {323, 459},   //  4 1/2" x 6 3/8"	114 x 162 mm
	"C7":  {230, 323},   // 3 3/16" x 4 1/2"	 81 x 114 mm
	"C8":  {162, 230},   //  2 1/4" x 3 3/16	 57 x 81 mm
	"C9":  {113, 162},   //  1 5/8" x 2 1/4"	 40 x 57 mm
	"C10": {79, 113},    //  1 1/8" x 1 5/8"	 28 x 40 mm

	// ISO 217:2013 untrimmed raw paper
	"RA0": {2438, 3458}, // 33.9" x 48.0"		860 x 1220 mm
	"RA1": {1729, 2438}, // 24.0" x 33.9"		610 x 860 mm
	"RA2": {1219, 1729}, // 16.9" x 24.0"		430 x 610 mm
	"RA3": {865, 1219},  // 12.0" x 16.9"		305 x 430 mm
	"RA4": {610, 865},   //  8.5" x 12.0"		215 x 305 mm

	"SRA0": {2551, 3628}, // 35.4" x 50.4"		900 x 1280 mm
	"SRA1": {1814, 2551}, // 25.2" x 35.4"		640 x 900 mm
	"SRA2": {1276, 1814}, // 17.7" x 25.2"		450 x 640 mm
	"SRA3": {907, 1276},  // 12.6" x 17.7"		320 x 450 mm
	"SRA4": {638, 907},   //  8.9" x 12.6"		225 x 320 mm

	"SRA1+":  {2835, 4008}, // 26.0" x 36.2"	660 x 920 mm
	"SRA2+":  {1361, 1843}, // 18.9" x 25.6"	480 x 650 mm
	"SRA3+":  {907, 1304},  // 12.6" x 18.1"	320 x 460 mm
	"SRA3++": {2835, 4008}, // 12.6" x 18.3"	320 x 464 mm

	// American
	"SuperB": {936, 1368}, //    13" x 19"
	"B+":     {936, 1368},

	"Tabloid":      {791, 1225}, //    11" x 17" 		ANSIB, DobleCarta
	"ExtraTabloid": {865, 1296}, //    12" x 18"		ARCHB, Arch2
	"Ledger":       {1225, 791}, //    17" x 11" 		ANSIB
	"Legal":        {612, 1009}, // 8 1/2" x 14"

	"GovLegal": {612, 936}, // 8 1/2" x 13"
	"Oficio":   {612, 936},
	"Folio":    {612, 936},

	"Letter":         {612, 791}, // 8 1/2" x 11"		ANSIA
	"Carta":          {612, 791},
	"AmericanQuarto": {612, 791},

	"DobleCarta": {791, 1225}, // 11" x 17"			Tabloid, ANSIB

	"GovLetter": {576, 757}, //     8" x 10 1/2"
	"Executive": {522, 756}, // 7 1/4" x 10 1/2"

	"HalfLetter": {397, 612}, // 5 1/2" x 8 1/2"
	"Memo":       {397, 612},
	"Statement":  {397, 612},
	"Stationary": {397, 612},

	"JuniorLegal": {360, 576}, // 5" x 8"
	"IndexCard":   {360, 576},

	"Photo": {288, 432}, // 4" x 6"

	// ANSI/ASME Y14.1
	"ANSIA": {612, 791},   // 8 1/2" x 11" Letter, Carta, AmericanQuarto
	"ANSIB": {791, 1225},  //    11" x 17" Ledger, Tabloid, DobleCarta
	"ANSIC": {1225, 1585}, //    17" x 22"
	"ANSID": {1585, 2449}, //    22" x 34"
	"ANSIE": {2449, 3170}, //    34" x 44"
	"ANSIF": {2016, 2880}, //    28" x 40"

	// ANSI/ASME Y14.1 Architectural series
	"ARCHA":  {649, 865},   //  9" x 12"	Arch 1
	"ARCHB":  {865, 1296},  // 12" x 18"	Arch 2, ExtraTabloide
	"ARCHC":  {1296, 1729}, // 18" x 24"	Arch 3
	"ARCHD":  {1729, 2591}, // 24" x 36"	Arch 4
	"ARCHE":  {2591, 3456}, // 36" x 48"	Arch 6
	"ARCHE1": {2160, 3025}, // 30" x 42"	Arch 5
	"ARCHE2": {1871, 2736}, // 26" x 38"
	"ARCHE3": {1945, 2809}, // 27" x 39"

	"Arch1": {649, 865},   //  9" x 12"	ARCHA
	"Arch2": {865, 1296},  // 12" x 18"	ARCHB, ExtraTabloide
	"Arch3": {1296, 1729}, // 18" x 24"	ARCHC
	"Arch4": {1729, 2591}, // 24" x 36"	ARCHD
	"Arch5": {2160, 3025}, // 30" x 42"	ARCHE1
	"Arch6": {2591, 3456}, // 36" x 48"	ARCHE

	// American Uncut
	"Bond":  {1584, 1224}, //     22" x 17"
	"Book":  {2736, 1800}, //     38" x 25"
	"Cover": {1872, 1440}, //     26" x 20"
	"Index": {2196, 1836}, // 30 1/2" x 25 1/2"

	"Newsprint": {2592, 1728}, // 36" x 24"
	"Tissue":    {2592, 1728},

	"Offset": {2736, 1800}, // 38" x 25"
	"Text":   {2736, 1800},

	// English Uncut
	"Crown":          {1170, 1512}, // 16 1/4" x 21"
	"DoubleCrown":    {1440, 2160}, //     20" x 30"
	"Quad":           {2160, 2880}, //     30" x 40"
	"Demy":           {1242, 1620}, // 17 3/4" x 22 1/2"
	"DoubleDemy":     {1620, 2556}, // 22 1/2" x 35 1/2"
	"Medium":         {1314, 1656}, // 18 1/4" x 23"
	"Royal":          {1440, 1804}, //     20" x 25 1/16"
	"SuperRoyal":     {1512, 1944}, //     21" x 27"
	"DoublePott":     {1080, 1800}, //     15" x 25"
	"DoublePost":     {1368, 2196}, //     19" x 30 1/2"
	"Foolscap":       {972, 1224},  // 13 1/2" x 17"
	"DoubleFoolscap": {1224, 1944}, //     17" x 27"

	"F4": {595, 935}, // 8 1/4" x 13"

	// GB/T 148-1997 D Series China
	"D0": {2166, 3016}, // 29.9" x 41.9"	764 x 1064 mm
	"D1": {1508, 2155}, // 20.9" x 29.9"	532 x 760 mm
	"D2": {1077, 1497}, // 15.0" x 20.8"	380 x 528 mm
	"D3": {748, 1066},  // 10.4" x 14.8"	264 x 376 mm
	"D4": {533, 737},   //  7.4" x 10.2"	188 x 260 mm
	"D5": {369, 522},   //  5.1" x 7.2"		130 x 184 mm
	"D6": {261, 357},   //  3.6" x 5.0"		 92 x 126 mm

	"RD0": {2231, 3096}, // 31.0" x 43.0"	787 x 1092 mm
	"RD1": {1548, 2231}, // 21.5" x 31.0"	546 x 787 mm
	"RD2": {1114, 1548}, // 15.5" x 21.5"	393 x 546 mm
	"RD3": {774, 1114},  // 10.7" x 15.5"	273 x 393 mm
	"RD4": {556, 774},   //  7.7" x 10.7"	196 x 273 mm
	"RD5": {386, 556},   //  5.4" x 7.7"	136 x 196 mm
	"RD6": {278, 386},   //  3.9" x 5.4"	 98 x 136 mm

	// Japanese B-series variant
	"JIS-B0":      {2920, 4127}, // 40.55" x 57.32"		1030 x 1456 mm
	"JIS-B1":      {2064, 2920}, // 28.66" x 40.55"	 	 728 x 1030 mm
	"JIS-B2":      {1460, 2064}, // 20.28" x 28.66"	 	 515 x 728 mm
	"JIS-B3":      {1032, 1460}, // 14.33" x 20.28"	 	 364 x 515 mm
	"JIS-B4":      {729, 1032},  // 10.12" x 14.33"	 	 257 x 364 mm
	"JIS-B5":      {516, 729},   //  7.17" x 10.12"	 	 182 x 257 mm
	"JIS-B6":      {363, 516},   //  5.04" x 7.17"		 128 x 182 mm
	"JIS-B7":      {258, 363},   //  3.58" x 5.04"		  91 x 128 mm
	"JIS-B8":      {181, 258},   //  2.52" x 3.58"		  64 x 91 mm
	"JIS-B9":      {127, 181},   //  1.77" x 2.52"		  45 x 64 mm
	"JIS-B10":     {91, 127},    //  1.26" x 1.77"		  32 x 45 mm
	"JIS-B11":     {63, 91},     //  0.87" x 1.26"		  22 x 32 mm
	"JIS-B12":     {45, 63},     //  0.63" x 0.87"		  16 x 22 mm
	"Shirokuban4": {748, 1074},  // 10.39" x 14.92"		 264 x 379 mm
	"Shirokuban5": {536, 742},   //  7.44" x 10.31"		 189 x 262 mm
	"Shirokuban6": {360, 533},   //  5.00" x 7.40"		 127 x 188 mm
	"Kiku4":       {644, 868},   //  8.94" x 12.05"		 227 x 306 mm
	"Kiku5":       {428, 644},   //  5.95" x 8.94"		 151 x 227 mm
	"AB":          {595, 729},   //  8.27" x 10.12"	 	 210 x 257 mm
	"B40":         {292, 516},   //  4.06" x 7.17"		 103 x 182 mm
	"Shikisen":    {238, 420},   //  3.31" x 5.83"		  84 x 148 mm
}
