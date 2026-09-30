//go:build windows

// 二维码编码器：字节模式、纠错等级 L、版本 1..10，只用标准库。
// 之所以自己写：整个项目的硬约束是"零第三方依赖、不联网也能构建"。
// 正确性由 qr_test.go 对着参考实现（Python segno）逐模块校验，不是靠肉眼。
package main

import "fmt"

// ---------------------------------------------------------------- 版本表

// 纠错等级 L 的分块参数（ISO/IEC 18004 表 9）。
type qrVersion struct {
	num        int
	ecPerBlock int   // 每块的纠错码字数
	blocks1    int   // 第 1 组块数
	data1      int   // 第 1 组每块数据码字数
	blocks2    int   // 第 2 组块数
	data2      int   // 第 2 组每块数据码字数
	align      []int // 对齐图案中心坐标
}

var qrVersions = [11]qrVersion{
	{},
	{num: 1, ecPerBlock: 7, blocks1: 1, data1: 19},
	{num: 2, ecPerBlock: 10, blocks1: 1, data1: 34, align: []int{6, 18}},
	{num: 3, ecPerBlock: 15, blocks1: 1, data1: 55, align: []int{6, 22}},
	{num: 4, ecPerBlock: 20, blocks1: 1, data1: 80, align: []int{6, 26}},
	{num: 5, ecPerBlock: 26, blocks1: 1, data1: 108, align: []int{6, 30}},
	{num: 6, ecPerBlock: 18, blocks1: 2, data1: 68, align: []int{6, 34}},
	{num: 7, ecPerBlock: 20, blocks1: 2, data1: 78, align: []int{6, 22, 38}},
	{num: 8, ecPerBlock: 24, blocks1: 2, data1: 97, align: []int{6, 24, 42}},
	{num: 9, ecPerBlock: 30, blocks1: 2, data1: 116, align: []int{6, 26, 46}},
	{num: 10, ecPerBlock: 18, blocks1: 2, data1: 68, blocks2: 2, data2: 69, align: []int{6, 28, 50}},
}

const qrMaxVersion = 10

func (v qrVersion) size() int          { return 17 + 4*v.num }
func (v qrVersion) dataCodewords() int { return v.blocks1*v.data1 + v.blocks2*v.data2 }
func (v qrVersion) totalCodewords() int {
	return v.dataCodewords() + (v.blocks1+v.blocks2)*v.ecPerBlock
}

// ---------------------------------------------------------------- 伽罗华域

var (
	gfExp [512]byte
	gfLog [256]byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D // QR 用的是本原多项式 x^8+x^4+x^3+x^2+1
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// rsGeneratorPoly 生成 (x-α^0)(x-α^1)...(x-α^(n-1))，下标 0 是最高次项。
func rsGeneratorPoly(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(g)+1)
		for j := 0; j < len(g); j++ {
			next[j] ^= g[j]
			next[j+1] ^= gfMul(g[j], gfExp[i])
		}
		g = next
	}
	return g
}

// rsEncode 返回 data 的 ecLen 个纠错码字（多项式除法取余）。
func rsEncode(data []byte, ecLen int) []byte {
	gen := rsGeneratorPoly(ecLen)
	rem := make([]byte, ecLen)
	for _, b := range data {
		factor := b ^ rem[0]
		copy(rem, rem[1:])
		rem[ecLen-1] = 0
		for i := 0; i < ecLen; i++ {
			rem[i] ^= gfMul(gen[i+1], factor)
		}
	}
	return rem
}

// ---------------------------------------------------------------- 位流编码

type bitBuffer struct{ bits []bool }

func (b *bitBuffer) put(value, n int) {
	for i := n - 1; i >= 0; i-- {
		b.bits = append(b.bits, (value>>i)&1 == 1)
	}
}

// qrEncodeData 产出最终交给矩阵的码字序列（数据 + 纠错，已交错）。
func qrEncodeData(payload []byte, ver int) ([]byte, error) {
	v := qrVersions[ver]
	capacity := v.dataCodewords() * 8
	var bb bitBuffer
	bb.put(4, 4) // 字节模式
	if ver >= 10 {
		bb.put(len(payload), 16)
	} else {
		bb.put(len(payload), 8)
	}
	for _, b := range payload {
		bb.put(int(b), 8)
	}
	if len(bb.bits) > capacity {
		return nil, fmt.Errorf("内容 %d 字节超出 %d 版容量", len(payload), ver)
	}
	// 终止符 + 补齐到字节边界
	for i := 0; i < 4 && len(bb.bits) < capacity; i++ {
		bb.bits = append(bb.bits, false)
	}
	for len(bb.bits)%8 != 0 {
		bb.bits = append(bb.bits, false)
	}
	out := make([]byte, 0, v.dataCodewords())
	for i := 0; i < len(bb.bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			if bb.bits[i+j] {
				b |= 1 << (7 - j)
			}
		}
		out = append(out, b)
	}
	// 交替填充 0xEC / 0x11
	for i := 0; len(out) < v.dataCodewords(); i++ {
		if i%2 == 0 {
			out = append(out, 0xEC)
		} else {
			out = append(out, 0x11)
		}
	}

	// 分块 -> 各自算纠错 -> 交错
	var dataBlocks [][]byte
	idx := 0
	for i := 0; i < v.blocks1; i++ {
		dataBlocks = append(dataBlocks, out[idx:idx+v.data1])
		idx += v.data1
	}
	for i := 0; i < v.blocks2; i++ {
		dataBlocks = append(dataBlocks, out[idx:idx+v.data2])
		idx += v.data2
	}
	ecBlocks := make([][]byte, len(dataBlocks))
	for i, db := range dataBlocks {
		ecBlocks[i] = rsEncode(db, v.ecPerBlock)
	}
	maxData := v.data1
	if v.data2 > maxData {
		maxData = v.data2
	}
	res := make([]byte, 0, v.totalCodewords())
	for i := 0; i < maxData; i++ {
		for _, db := range dataBlocks {
			if i < len(db) {
				res = append(res, db[i])
			}
		}
	}
	for i := 0; i < v.ecPerBlock; i++ {
		for _, eb := range ecBlocks {
			res = append(res, eb[i])
		}
	}
	return res, nil
}

// ---------------------------------------------------------------- 矩阵构建

type qrMatrix struct {
	size     int
	dark     []bool // true = 深色模块
	reserved []bool // 功能图案占用，数据不能写
}

func newQRMatrix(ver int) *qrMatrix {
	n := qrVersions[ver].size()
	return &qrMatrix{size: n, dark: make([]bool, n*n), reserved: make([]bool, n*n)}
}

func (m *qrMatrix) at(x, y int) int { return y*m.size + x }

func (m *qrMatrix) set(x, y int, dark bool) {
	m.dark[m.at(x, y)] = dark
	m.reserved[m.at(x, y)] = true
}

func (m *qrMatrix) get(x, y int) bool { return m.dark[m.at(x, y)] }

func (m *qrMatrix) isReserved(x, y int) bool { return m.reserved[m.at(x, y)] }

// buildFunctionPatterns 画定位/定时/校正/固定深色模块，并占位格式信息区域。
func (m *qrMatrix) buildFunctionPatterns(ver int) {
	m.placeFinder(0, 0)
	m.placeFinder(m.size-7, 0)
	m.placeFinder(0, m.size-7)
	for i := 8; i < m.size-8; i++ {
		m.set(i, 6, i%2 == 0)
		m.set(6, i, i%2 == 0)
	}
	coords := qrVersions[ver].align
	for _, cy := range coords {
		for _, cx := range coords {
			if (cx == 6 && cy == 6) || (cx == 6 && cy == m.size-7) || (cx == m.size-7 && cy == 6) {
				continue
			}
			m.placeAlignment(cx, cy)
		}
	}
	// 格式信息占位（第一份 + 第二份），后面用真实位覆盖
	for i := 0; i < 9; i++ {
		if i != 6 {
			m.set(8, i, false)
			m.set(i, 8, false)
		}
	}
	for i := 0; i < 8; i++ {
		m.set(m.size-1-i, 8, false)
	}
	for i := 0; i < 7; i++ {
		m.set(8, m.size-1-i, false)
	}
	m.set(8, m.size-8, true) // 固定深色模块
	if ver >= 7 {
		for i := 0; i < 18; i++ {
			m.set(m.size-11+i%3, i/3, false)
			m.set(i/3, m.size-11+i%3, false)
		}
	}
}

func (m *qrMatrix) placeFinder(cx, cy int) {
	for dy := -1; dy <= 7; dy++ {
		for dx := -1; dx <= 7; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= m.size || y >= m.size {
				continue
			}
			inner := dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4
			ring := dx >= 0 && dx <= 6 && (dy == 0 || dy == 6)
			side := dy >= 0 && dy <= 6 && (dx == 0 || dx == 6)
			m.set(x, y, inner || ring || side)
		}
	}
}

func (m *qrMatrix) placeAlignment(cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			edge := dx == -2 || dx == 2 || dy == -2 || dy == 2
			m.set(cx+dx, cy+dy, edge || (dx == 0 && dy == 0))
		}
	}
}

// placeData 按"从右下角起、每次两列、蛇形上下"的规则填数据位。
func (m *qrMatrix) placeData(bits []bool) {
	i := 0
	up := true
	for right := m.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // 第 6 列是垂直定时图案，跳过
		}
		for j := 0; j < m.size; j++ {
			y := j
			if up {
				y = m.size - 1 - j
			}
			for _, x := range []int{right, right - 1} {
				if m.isReserved(x, y) {
					continue
				}
				dark := false
				if i < len(bits) {
					dark = bits[i]
				}
				i++
				m.dark[m.at(x, y)] = dark
			}
		}
		up = !up
	}
}

// ---------------------------------------------------------------- 掩码/格式

func maskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return (x*y)%2+(x*y)%3 == 0
	case 6:
		return ((x*y)%2+(x*y)%3)%2 == 0
	default:
		return ((x+y)%2+(x*y)%3)%2 == 0
	}
}

// bch 计算 BCH 余数：data 左移 polyBits 位后对 gen 取余。
func bch(data, gen, genBits int) int {
	d := data << genBits
	for i := genBits + bitsLen(data) - 1; i >= genBits; i-- {
		if d&(1<<i) != 0 {
			d ^= gen << (i - genBits)
		}
	}
	return d
}

func bitsLen(v int) int {
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

// formatBits 生成 15 位格式信息（纠错等级 L = 0b01）。
func formatBits(mask int) int {
	data := 0b01<<3 | mask
	return (data<<10 | bch(data, 0x537, 10)) ^ 0x5412
}

// versionBits 生成 18 位版本信息（版本 >= 7 才有）。
func versionBits(ver int) int {
	return ver<<12 | bch(ver, 0x1F25, 12)
}

func (m *qrMatrix) applyFormat(mask int) {
	bits := formatBits(mask)
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	for i := 0; i < 15; i++ {
		d := bit(i)
		switch {
		case i < 6:
			m.dark[m.at(8, i)] = d
		case i < 8:
			m.dark[m.at(8, i+1)] = d
		default:
			m.dark[m.at(8, m.size-15+i)] = d
		}
		switch {
		case i < 8:
			m.dark[m.at(m.size-1-i, 8)] = d
		case i == 8:
			m.dark[m.at(7, 8)] = d
		default:
			m.dark[m.at(14-i, 8)] = d
		}
	}
	m.dark[m.at(8, m.size-8)] = true
}

func (m *qrMatrix) applyVersion(ver int) {
	if ver < 7 {
		return
	}
	bits := versionBits(ver)
	for i := 0; i < 18; i++ {
		d := (bits>>i)&1 == 1
		m.dark[m.at(m.size-11+i%3, i/3)] = d
		m.dark[m.at(i/3, m.size-11+i%3)] = d
	}
}

func (m *qrMatrix) applyMask(mask int) {
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if m.isReserved(x, y) {
				continue
			}
			if maskBit(mask, x, y) {
				m.dark[m.at(x, y)] = !m.dark[m.at(x, y)]
			}
		}
	}
}

// ---------------------------------------------------------------- 掩码评分

func (m *qrMatrix) penalty() int {
	score := 0
	n := m.size
	// 规则 1：同行/同列连续同色 >= 5
	for y := 0; y < n; y++ {
		score += runPenalty(func(i int) bool { return m.get(i, y) }, n)
	}
	for x := 0; x < n; x++ {
		score += runPenalty(func(i int) bool { return m.get(x, i) }, n)
	}
	// 规则 2：2x2 同色块
	for y := 0; y < n-1; y++ {
		for x := 0; x < n-1; x++ {
			c := m.get(x, y)
			if c == m.get(x+1, y) && c == m.get(x, y+1) && c == m.get(x+1, y+1) {
				score += 3
			}
		}
	}
	// 规则 3：形如 1011101 0000 / 0000 1011101 的组合。
	// 两侧都是 4 个浅色模块时会各记一次（共 80 分），这是 Nayuki 等主流实现的做法；
	// segno 按“每处图案一次”只记 40 分。两种口径都是合法的，影响的只是自动选掩码，
	// 8 种掩码都能扫出同样的内容。
	pat1 := []bool{true, false, true, true, true, false, true, false, false, false, false}
	pat2 := []bool{false, false, false, false, true, false, true, true, true, false, true}
	for y := 0; y < n; y++ {
		line := make([]bool, n)
		for x := 0; x < n; x++ {
			line[x] = m.get(x, y)
		}
		score += countPattern(line, pat1) * 40
		score += countPattern(line, pat2) * 40
	}
	for x := 0; x < n; x++ {
		line := make([]bool, n)
		for y := 0; y < n; y++ {
			line[y] = m.get(x, y)
		}
		score += countPattern(line, pat1) * 40
		score += countPattern(line, pat2) * 40
	}
	// 规则 4：深色比例偏离 50% 的程度
	dark := 0
	for _, d := range m.dark {
		if d {
			dark++
		}
	}
	percent := dark * 100 / (n * n)
	dev := percent - 50
	if dev < 0 {
		dev = -dev
	}
	score += dev / 5 * 10
	return score
}

func runPenalty(get func(int) bool, n int) int {
	score := 0
	run := 1
	for i := 1; i < n; i++ {
		if get(i) == get(i-1) {
			run++
			continue
		}
		if run >= 5 {
			score += 3 + (run - 5)
		}
		run = 1
	}
	if run >= 5 {
		score += 3 + (run - 5)
	}
	return score
}

func countPattern(line, pat []bool) int {
	count := 0
	for i := 0; i+len(pat) <= len(line); i++ {
		ok := true
		for j := range pat {
			if line[i+j] != pat[j] {
				ok = false
				break
			}
		}
		if ok {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------- 对外接口

// qrMake 返回二维码模块矩阵（true = 深色），不含静默区。
// forceMask < 0 表示按评分自动选掩码。
func qrMake(payload string, forceMask int) (*qrMatrix, int, error) {
	data := []byte(payload)
	ver := 0
	for v := 1; v <= qrMaxVersion; v++ {
		if _, err := qrEncodeData(data, v); err == nil {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, 0, fmt.Errorf("内容太长（%d 字节），超出 %d 版容量", len(data), qrMaxVersion)
	}
	return qrMakeWithVersion(payload, ver, forceMask)
}

func qrMakeWithVersion(payload string, ver, forceMask int) (*qrMatrix, int, error) {
	words, err := qrEncodeData([]byte(payload), ver)
	if err != nil {
		return nil, 0, err
	}
	bits := make([]bool, 0, len(words)*8)
	for _, w := range words {
		for i := 7; i >= 0; i-- {
			bits = append(bits, (w>>i)&1 == 1)
		}
	}

	base := newQRMatrix(ver)
	base.buildFunctionPatterns(ver)
	base.placeData(bits)

	bestMask, bestScore := 0, -1
	if forceMask >= 0 {
		bestMask = forceMask
	} else {
		for mask := 0; mask < 8; mask++ {
			cand := base.clone()
			cand.applyMask(mask)
			cand.applyFormat(mask)
			cand.applyVersion(ver)
			if s := cand.penalty(); bestScore < 0 || s < bestScore {
				bestMask, bestScore = mask, s
			}
		}
	}
	out := base.clone()
	out.applyMask(bestMask)
	out.applyFormat(bestMask)
	out.applyVersion(ver)
	return out, bestMask, nil
}

func (m *qrMatrix) clone() *qrMatrix {
	c := &qrMatrix{size: m.size, dark: make([]bool, len(m.dark)), reserved: make([]bool, len(m.reserved))}
	copy(c.dark, m.dark)
	copy(c.reserved, m.reserved)
	return c
}
