package qrpng

// Pure-stdlib QR encoder + PNG renderer with a small red radiation logo overlay.
// Design (why this shape):
// - Byte mode only: simplest and universal for URLs.
// - EC level M, versions 1..20 auto-fit: balances capacity and central logo tolerance.
// - Reed–Solomon (GF(256), prim poly 0x11D), generator built per ecLen on the fly.
// - Proper function patterns, format BCH(15,5) (xor 0x5412), version BCH(18,6) for ver>=7.
// - Mask selection 0..7 by ISO penalty score.
// - Rendering to PNG with a 4-module quiet zone.
// - Concurrency w/out mutexes: per-block ECC computed in goroutines, gathered via a channel.
// Go proverbs: "Don't communicate by sharing memory; share memory by communicating."

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"io"
)

// Public API -----------------------------------------------------------------

// EncodePNG writes a PNG with black QR on white background and a small red
// radiation symbol in the center. It auto-picks version 1..20 at EC=M.
// Returns an error if data doesn't fit into V20-M.
func EncodePNG(w io.Writer, data []byte) error {
	m, size, err := buildQR(data)
	if err != nil {
		return err
	}
	img := renderQRToPNG(m, size)
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	return enc.Encode(w, img)
}

// Internals: QR build pipeline ------------------------------------------------

func buildQR(data []byte) (matrix [][]bool, n int, err error) {
	for ver := 1; ver <= 20; ver++ {
		totalData, ecPerBlock, g1n, g1k, g2n, g2k := ecParamsM(ver)
		if totalData == 0 {
			continue
		}
		bits, err := makeBitStreamByteMode(data, ver)
		if err != nil {
			return nil, 0, err
		}
		dcw := bitsToBytesAndPad(bits, totalData)
		msg := interleaveWithECC(dcw, ecPerBlock, g1n, g1k, g2n, g2k)

		size := 17 + 4*ver
		base, isFunc := makeBaseMatrix(ver, size)
		fillData(base, isFunc, msg)

		best := copyMatrix(base)
		bestMask := -1
		bestScore := math.MaxInt
		for mask := 0; mask < 8; mask++ {
			mm := copyMatrix(base)
			applyMask(mm, isFunc, mask)
			putFormatInfo(mm, mask, 'M')
			if ver >= 7 {
				putVersionInfo(mm, ver)
			}
			score := penaltyScore(mm)
			if score < bestScore {
				bestScore = score
				bestMask = mask
				best = mm
			}
		}
		if bestMask >= 0 {
			return best, size, nil
		}
	}
	return nil, 0, fmt.Errorf("data too long for version <=20 at EC=M")
}

// Bitstream (Byte mode) ------------------------------------------------------

func makeBitStreamByteMode(data []byte, version int) ([]bool, error) {
	out := make([]bool, 0, len(data)*8+32)
	// mode indicator 0100
	out = appendBits(out, 0b0100, 4)
	// count
	if version <= 9 {
		if len(data) > 255 {
			return nil, fmt.Errorf("too long for v<=9")
		}
		out = appendBits(out, len(data), 8)
	} else {
		if len(data) > 65535 {
			return nil, fmt.Errorf("too long")
		}
		out = appendBits(out, len(data), 16)
	}
	for _, b := range data {
		out = appendBits(out, int(b), 8)
	}
	return out, nil
}

func appendBits(bits []bool, val, n int) []bool {
	for i := n - 1; i >= 0; i-- {
		bits = append(bits, ((val>>i)&1) == 1)
	}
	return bits
}

func bitsToBytesAndPad(bits []bool, capBytes int) []byte {
	// terminator up to 4 bits
	remBits := capBytes*8 - len(bits)
	t := 4
	if remBits < 4 {
		t = remBits
	}
	for i := 0; i < t; i++ {
		bits = append(bits, false)
	}
	// align to byte
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	out := make([]byte, len(bits)/8)
	for i := range out {
		var b byte
		for j := 0; j < 8; j++ {
			if bits[i*8+j] {
				b |= 1 << (7 - j)
			}
		}
		out[i] = b
	}
	// pad bytes 0xEC, 0x11
	pads := capBytes - len(out)
	p := []byte{0xEC, 0x11}
	for i := 0; i < pads; i++ {
		out = append(out, p[i%2])
	}
	return out
}

// ECC + interleaving (channels, no mutexes) ----------------------------------

func interleaveWithECC(data []byte, ecPerBlock, g1n, g1k, g2n, g2k int) []byte {
	blocks := splitBlocks(data, g1n, g1k, g2n, g2k)

	type res struct{ i int; ecc []byte }
	ch := make(chan res, len(blocks))
	for i := range blocks {
		i := i
		go func() { ch <- res{i, rsECC(blocks[i], ecPerBlock)} }()
	}
	ec := make([][]byte, len(blocks))
	for i := 0; i < len(blocks); i++ {
		r := <-ch
		ec[r.i] = r.ecc
	}

	// interleave data
	maxD := 0
	for _, b := range blocks {
		if len(b) > maxD {
			maxD = len(b)
		}
	}
	out := make([]byte, 0, maxD*len(blocks))
	for i := 0; i < maxD; i++ {
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	// interleave ecc
	maxE := 0
	for _, e := range ec {
		if len(e) > maxE {
			maxE = len(e)
		}
	}
	for i := 0; i < maxE; i++ {
	 for _, e := range ec {
		 if i < len(e) { out = append(out, e[i]) }
	 }
	}
	return out
}

func splitBlocks(data []byte, g1n, g1k, g2n, g2k int) [][]byte {
	blocks := make([][]byte, 0, g1n+g2n)
	off := 0
	for i := 0; i < g1n; i++ {
		blocks = append(blocks, append([]byte(nil), data[off:off+g1k]...))
		off += g1k
	}
	for i := 0; i < g2n; i++ {
		blocks = append(blocks, append([]byte(nil), data[off:off+g2k]...))
		off += g2k
	}
	return blocks
}

// GF(256) + Reed–Solomon -----------------------------------------------------

func rsECC(data []byte, ecLen int) []byte {
	if ecLen <= 0 {
		return nil
	}
	alog, log := gfTables()
	// generator = Π (x - a^i), i=0..ecLen-1
	gen := []int{1}
	for i := 0; i < ecLen; i++ {
		gen = polyMul(gen, []int{1, gfPow(alog, log, 2, i)}, alog, log)
	}
	msg := make([]int, len(data)+ecLen)
	for i := range data {
		msg[i] = int(data[i])
	}
	for i := 0; i < len(data); i++ {
		if msg[i] == 0 {
			continue
		}
		f := msg[i]
		for j := 0; j < len(gen); j++ {
			msg[i+j] ^= gfMul(alog, log, f, gen[j])
		}
	}
	ecc := make([]byte, ecLen)
	for i := 0; i < ecLen; i++ {
		ecc[i] = byte(msg[len(data)+i])
	}
	return ecc
}

func gfTables() ([]int, []int) {
	alog := make([]int, 512)
	log := make([]int, 256)
	x := 1
	for i := 0; i < 255; i++ {
		alog[i] = x
		log[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		alog[i] = alog[i-255]
	}
	return alog, log
}
func gfMul(alog, log []int, a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return alog[(log[a]+log[b])%255]
}
func gfPow(alog, log []int, a, n int) int {
	if a == 0 {
		return 0
	}
	return alog[(log[a]*n)%255]
}
func polyMul(a, b []int, alog, log []int) []int {
	out := make([]int, len(a)+len(b)-1)
	for i := 0; i < len(a); i++ {
		for j := 0; j < len(b); j++ {
			out[i+j] ^= gfMul(alog, log, a[i], b[j])
		}
	}
	return out
}

// Matrix patterns, data placement, masking -----------------------------------

func makeBaseMatrix(version, size int) (m [][]bool, isFunc [][]bool) {
	m = make([][]bool, size)
	isFunc = make([][]bool, size)
	for y := 0; y < size; y++ {
		m[y] = make([]bool, size)
		isFunc[y] = make([]bool, size)
	}
	// finders + separators
	putFinder(m, isFunc, 0, 0)
	putFinder(m, isFunc, size-7, 0)
	putFinder(m, isFunc, 0, size-7)
	// timing
	for i := 8; i < size-8; i++ {
		m[6][i] = (i%2 == 0); isFunc[6][i] = true
		m[i][6] = (i%2 == 0); isFunc[i][6] = true
	}
	// alignment
	apos := alignmentPositions(version)
	for i := range apos {
		for j := range apos {
			ax, ay := apos[i], apos[j]
			if (ax <= 7 && ay <= 7) || (ax >= size-8 && ay <= 7) || (ax <= 7 && ay >= size-8) {
				continue
			}
			putAlignment(m, isFunc, ax-2, ay-2)
		}
	}
	// dark module
	m[4*version+9][8] = true
	isFunc[4*version+9][8] = true
	// reserve format info
	for i := 0; i < 9; i++ {
		if i != 6 {
			isFunc[8][i] = true
			isFunc[i][8] = true
			isFunc[8][size-1-i] = true
			isFunc[size-1-i][8] = true
		}
	}
	// reserve version info (>=7)
	if version >= 7 {
		for i := 0; i < 6; i++ {
			for j := 0; j < 3; j++ {
				isFunc[size-11+j][i] = true
				isFunc[i][size-11+j] = true
			}
		}
	}
	return
}

func putFinder(m, isFunc [][]bool, x, y int) {
	size := len(m)

	// 7x7 finder (with inner 3x3 dark)
	// Bounds-checked to avoid overruns near edges.
	for dy := 0; dy < 7; dy++ {
		yy := y + dy
		if yy < 0 || yy >= size {
			continue
		}
		for dx := 0; dx < 7; dx++ {
			xx := x + dx
			if xx < 0 || xx >= size {
				continue
			}
			v := (dx == 0 || dx == 6 || dy == 0 || dy == 6) || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4)
			m[yy][xx] = v
			isFunc[yy][xx] = true
		}
	}

	// Separators (one-pixel white border around finder).
	// Guard every index on both axes to avoid x+i==size or y+i==size writes.
	for i := 0; i < 8; i++ {
		yi := y + i
		xi := x + i

		// left separator column at x-1
		if x-1 >= 0 && yi >= 0 && yi < size {
			m[yi][x-1] = false
			isFunc[yi][x-1] = true
		}
		// right separator column at x+7
		if x+7 < size && yi >= 0 && yi < size {
			m[yi][x+7] = false
			isFunc[yi][x+7] = true
		}
		// top separator row at y-1
		if y-1 >= 0 && xi >= 0 && xi < size {
			m[y-1][xi] = false
			isFunc[y-1][xi] = true
		}
		// bottom separator row at y+7
		if y+7 < size && xi >= 0 && xi < size {
			m[y+7][xi] = false
			isFunc[y+7][xi] = true
		}
	}
}

func putAlignment(m, isFunc [][]bool, x, y int) {
	for dy := 0; dy < 5; dy++ {
		for dx := 0; dx < 5; dx++ {
			ax := x + dx; ay := y + dy
			v := dx == 0 || dx == 4 || dy == 0 || dy == 4 || (dx == 2 && dy == 2)
			m[ay][ax] = v; isFunc[ay][ax] = true
		}
	}
}

func alignmentPositions(version int) []int {
	switch version {
	case 1:  return nil
	case 2:  return []int{6, 18}
	case 3:  return []int{6, 22}
	case 4:  return []int{6, 26}
	case 5:  return []int{6, 30}
	case 6:  return []int{6, 34}
	case 7:  return []int{6, 22, 38}
	case 8:  return []int{6, 24, 42}
	case 9:  return []int{6, 26, 46}
	case 10: return []int{6, 28, 50}
	case 11: return []int{6, 30, 54}
	case 12: return []int{6, 32, 58}
	case 13: return []int{6, 34, 62}
	case 14: return []int{6, 26, 46, 66}
	case 15: return []int{6, 26, 48, 70}
	case 16: return []int{6, 26, 50, 74}
	case 17: return []int{6, 30, 54, 78}
	case 18: return []int{6, 30, 56, 82}
	case 19: return []int{6, 30, 58, 86}
	case 20: return []int{6, 34, 62, 90}
	default: return nil
	}
}

// Zig-zag data placement (unmasked)
func fillData(m, isFunc [][]bool, msg []byte) {
	size := len(m)
	bit := 0
	for col := size - 1; col > 0; col -= 2 {
		if col == 6 { col-- } // skip timing col
		up := ((size - col) / 2) % 2 == 1
		if up {
			for row := 0; row < size; row++ {
				for c := col; c >= col-1; c-- {
					if isFunc[row][c] { continue }
					b := getBit(msg, bit); bit++
					m[row][c] = (b == 1)
				}
			}
		} else {
			for row := size - 1; row >= 0; row-- {
				for c := col; c >= col-1; c-- {
					if isFunc[row][c] { continue }
					b := getBit(msg, bit); bit++
					m[row][c] = (b == 1)
				}
			}
		}
	}
}

func getBit(msg []byte, i int) int {
	if i < 0 || i >= len(msg)*8 { return 0 }
	b := msg[i/8]
	return int((b >> (7 - (i%8))) & 1)
}

func copyMatrix(m [][]bool) [][]bool {
	size := len(m)
	out := make([][]bool, size)
	for i := range m { out[i] = append([]bool(nil), m[i]...) }
	return out
}

func applyMask(m, isFunc [][]bool, mask int) {
	size := len(m)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if isFunc[y][x] { continue }
			if maskCond(mask, x, y) { m[y][x] = !m[y][x] }
		}
	}
}

func maskCond(mask, x, y int) bool {
	switch mask {
	case 0: return (x+y)%2 == 0
	case 1: return y%2 == 0
	case 2: return x%3 == 0
	case 3: return (x+y)%3 == 0
	case 4: return ((y/2)+(x/3))%2 == 0
	case 5: return ((x*y)%2+ (x*y)%3) == 0
	case 6: return (((x*y)%2)+((x*y)%3))%2 == 0
	case 7: return (((x+y)%2)+((x*y)%3))%2 == 0
	default: return false
	}
}

// Penalty score (rules 1..4)
func penaltyScore(m [][]bool) int {
	size := len(m)
	score := 0
	// rule 1: rows
	for y := 0; y < size; y++ {
		run := 1
		for x := 1; x < size; x++ {
			if m[y][x] == m[y][x-1] { run++ } else {
				if run >= 5 { score += 3 + (run-5) }
				run = 1
			}
		}
		if run >= 5 { score += 3 + (run-5) }
	}
	// rule 1: cols
	for x := 0; x < size; x++ {
		run := 1
		for y := 1; y < size; y++ {
			if m[y][x] == m[y-1][x] { run++ } else {
				if run >= 5 { score += 3 + (run-5) }
				run = 1
			}
		}
		if run >= 5 { score += 3 + (run-5) }
	}
	// rule 2: 2x2 blocks
	for y := 0; y < size-1; y++ {
		for x := 0; x < size-1; x++ {
			v := m[y][x]
			if m[y][x+1]==v && m[y+1][x]==v && m[y+1][x+1]==v { score += 3 }
		}
	}
	// rule 3: finder-like pattern in rows/cols
	pA := []int{1,0,1,1,1,0,1,0,0,0,0}
	pB := []int{0,0,0,0,1,0,1,1,1,0,1}
	score += patternPenalty(m, pA)
	score += patternPenalty(m, pB)
	// rule 4: dark ratio
	total := size*size; dark:=0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ { if m[y][x] { dark++ } }
	}
	pct := (dark*100)/total
	k := abs(pct-50)/5
	score += k*10
	return score
}

func patternPenalty(m [][]bool, pat []int) int {
	size := len(m); pen := 0
	for y := 0; y < size; y++ {
		row := make([]int, size)
		for x := 0; x < size; x++ { if m[y][x] { row[x]=1 } }
		pen += countPattern(row, pat)*40
	}
	for x := 0; x < size; x++ {
		col := make([]int, size)
		for y := 0; y < size; y++ { if m[y][x] { col[y]=1 } }
		pen += countPattern(col, pat)*40
	}
	return pen
}
func countPattern(a, p []int) int {
	n:=0
	for i:=0; i+len(p)<=len(a); i++ {
		ok:=true
		for j:=0;j<len(p);j++ { if a[i+j]!=p[j]{ ok=false;break } }
		if ok { n++ }
	}
	return n
}
func abs(x int) int { if x<0 {return -x}; return x }

// Format & Version info ------------------------------------------------------

// putFormatInfo writes both 15-bit copies (BCH(15,5) + xor 0x5412).
// Bits b14..b0 are placed per spec. We map MSB first (b14) down to LSB (b0).
func putFormatInfo(m [][]bool, mask int, ec rune) {
	ec2 := 0 // L=01, M=00, Q=11, H=10
	switch ec {
	case 'L': ec2 = 0b01
	case 'M': ec2 = 0b00
	case 'Q': ec2 = 0b11
	case 'H': ec2 = 0b10
	}
	val := (ec2<<3) | (mask & 7)        // 5 bits
	code := bch15_5(val) ^ 0x5412       // 15 bits
	size := len(m)

	// Build bit array b[0]=b14(MSB)..b[14]=b0(LSB)
	b := make([]bool,15)
	for i:=0;i<15;i++ { b[i] = ((code>>(14-i))&1)==1 }

	// Copy 1 (around TL finder): row 8 and col 8 (skipping timing at (8,6)/(6,8))
	// row 8: positions (8,0..5),(8,7),(8,8)
	rowPos := [8][2]int{{8,0},{8,1},{8,2},{8,3},{8,4},{8,5},{8,7},{8,8}}
	for i:=0;i<8;i++ { m[rowPos[i][0]][rowPos[i][1]] = b[i] }
	// col 8: positions (0..5,8),(7..8,8)
	colPos := [7][2]int{{0,8},{1,8},{2,8},{3,8},{4,8},{5,8},{7,8}}
	for i:=0;i<7;i++ { m[colPos[i][0]][colPos[i][1]] = b[8+i] }

	// Copy 2 (near TR and BL)
	// col: (size-1,8) down to (size-7,8) -> b0..b6
	for i:=0;i<7;i++ {
		m[size-1-i][8] = b[14-i]
	}
	// row: (8,size-8) to (8,size-1) -> b7..b14
	for i:=0;i<8;i++ {
		m[8][size-8+i] = b[7-i]
	}
}

// BCH(15,5) with generator 0x537 (x^10 + x^8 + x^5 + x^4 + x^2 + x + 1)
func bch15_5(v int) int {
	g := 0b10100110111
	val := v << 10
	for i:=14;i>=10;i-- {
		if (val>>i)&1==1 { val ^= g<<(i-10) }
	}
	return (v<<10) | (val & 0x3FF)
}

// Version info for ver>=7: BCH(18,6) (gen 0b1111100100101)
func putVersionInfo(m [][]bool, ver int) {
	gen := 0b1111100100101
	v := ver<<12
	tmp := v
	for i:=17;i>=12;i-- {
		if (tmp>>i)&1==1 { tmp ^= gen<<(i-12) }
	}
	code := v | (tmp & 0xFFF)
	size := len(m)
	// bottom-left 3x6
	for i:=0;i<18;i++ {
		b := ((code>>(17-i))&1)==1
		x := i%3; y := i/3
		m[size-11+y][x] = b
	}
	// top-right 6x3
	for i:=0;i<18;i++ {
		b := ((code>>(17-i))&1)==1
		x := i/3; y := i%3
		m[y][size-11+x] = b
	}
}

// EC params (versions 1..20, EC=M): total data bytes, ec/blk, group1 n/k, group2 n/k
func ecParamsM(version int) (totalData, ecPerBlock, g1n, g1k, g2n, g2k int) {
	switch version {
	case 1:  return 16, 10, 1, 16, 0, 0
	case 2:  return 28, 16, 1, 28, 0, 0
	case 3:  return 44, 26, 1, 44, 0, 0
	case 4:  return 64, 18, 2, 32, 0, 0
	case 5:  return 86, 24, 2, 43, 0, 0
	case 6:  return 108,16, 4, 27, 0, 0
	case 7:  return 124,18, 4, 31, 0, 0
	case 8:  return 154,22, 2, 38, 2, 39
	case 9:  return 182,22, 3, 36, 2, 37
	case 10: return 216,26, 4, 43, 1, 44
	case 11: return 254,30, 1, 50, 4, 51
	case 12: return 290,22, 6, 36, 2, 37
	case 13: return 334,22, 8, 37, 1, 38
	case 14: return 365,24, 4, 40, 5, 41
	case 15: return 415,24, 5, 41, 5, 42
	case 16: return 453,28, 7, 45, 3, 46
	case 17: return 507,28,10, 46, 1, 47
	case 18: return 563,26, 9, 43, 4, 44
	case 19: return 627,26, 3, 44,11, 45
	case 20: return 669,26, 3, 41,13, 42
	default: return 0,0,0,0,0,0
	}
}

// Rendering: QR -> PNG + radiation overlay ----------------------------------

func renderQRToPNG(m [][]bool, size int) image.Image {
	quiet := 4
	scale := chooseScale(size, 1000)
	W := (size + 2*quiet) * scale
	dst := image.NewRGBA(image.Rect(0, 0, W, W))
	white := color.RGBA{255,255,255,255}
	draw.Draw(dst, dst.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	black := color.RGBA{0,0,0,255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if m[y][x] {
				px := (x+quiet)*scale
				py := (y+quiet)*scale
				fillRect(dst, px, py, scale, scale, black)
			}
		}
	}

	// small red radiation sign in center (kept small; EC=M tolerates it)
	cx := W/2; cy := W/2
	rOuter := int(0.12 * float64(W))
	rInner := int(0.55 * float64(rOuter))
	rCenter:= int(0.28 * float64(rOuter))
	red := color.RGBA{220,0,0,255}
	drawWedge(dst, cx, cy, rInner, rOuter, deg(90-30),  deg(90+30),  red)
	drawWedge(dst, cx, cy, rInner, rOuter, deg(210-30), deg(210+30), red)
	drawWedge(dst, cx, cy, rInner, rOuter, deg(330-30), deg(330+30), red)
	fillCircle(dst, cx, cy, rCenter, red)
	return dst
}

func chooseScale(n, target int) int {
	s := target / (n + 8)
	if s < 4 { s = 4 }
	if s > 16 { s = 16 }
	return s
}

func fillRect(img *image.RGBA, x, y, w, h int, col color.Color) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			img.Set(xx, yy, col)
		}
	}
}

func deg(d float64) float64 { return d * (math.Pi / 180) }

func fillCircle(img *image.RGBA, cx, cy, r int, col color.Color) {
	if r <= 0 { return }
	r2 := r*r
	b := img.Bounds()
	minY := max(cy-r, b.Min.Y)
	maxY := min(cy+r, b.Max.Y-1)
	for y := minY; y <= maxY; y++ {
		dy := y - cy
		xx := int(math.Sqrt(float64(r2 - dy*dy)))
		x1 := max(cx-xx, b.Min.X)
		x2 := min(cx+xx, b.Max.X-1)
		for x := x1; x <= x2; x++ {
			img.Set(x, y, col)
		}
	}
}

func drawWedge(img *image.RGBA, cx, cy, ri, ro int, a0, a1 float64, col color.Color) {
	if ro <= ri || ro <= 0 { return }
	for a0 < 0 { a0 += 2*math.Pi }
	for a1 < 0 { a1 += 2*math.Pi }
	for a0 >= 2*math.Pi { a0 -= 2*math.Pi }
	for a1 >= 2*math.Pi { a1 -= 2*math.Pi }
	in := func(a float64) bool {
		if a0 <= a1 { return a >= a0 && a <= a1 }
		return a >= a0 || a <= a1
	}
	b := img.Bounds()
	minX := max(cx-ro, b.Min.X); maxX := min(cx+ro, b.Max.X-1)
	minY := max(cy-ro, b.Min.Y); maxY := min(cy+ro, b.Max.Y-1)
	ro2 := ro*ro; ri2 := ri*ri
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx := x - cx; dy := y - cy
			r2 := dx*dx + dy*dy
			if r2 < ri2 || r2 > ro2 { continue }
			a := math.Atan2(float64(dy), float64(dx)); if a < 0 { a += 2*math.Pi }
			if in(a) { img.Set(x, y, col) }
		}
	}
}

func min(a,b int) int { if a<b {return a}; return b }
func max(a,b int) int { if a>b {return a}; return b }

