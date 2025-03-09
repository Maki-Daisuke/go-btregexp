// Package btregexp は、バックトラック型の正規表現エンジンを実装したパッケージです。
package btregexp

import (
	"io"
)

// Matcher は、正規表現マッチングエンジンを表します。
type Matcher struct {
	prog            *program // コンパイルされた正規表現プログラム
	input           []rune   // 入力文字列（Unicodeルーン配列）
	pos             int      // 現在の入力位置
	multiline       bool     // マルチラインモード
	caseInsensitive bool     // 大文字小文字を区別しない
	dotMatchesNL    bool     // ドットが改行にマッチする
	startPos        int      // マッチ開始位置
	captures        [][]int  // キャプチャグループの位置
	saved           []int    // 保存された位置
	maxSteps        int      // 最大実行ステップ数（無限ループ防止）
	steps           int      // 現在の実行ステップ数
}

// BacktrackPoint は、バックトラックするポイントを表します。
type BacktrackPoint struct {
	pc       int   // プログラムカウンタ
	pos      int   // 入力位置
	captures []int // キャプチャ状態
}

// newMatcher は、新しいマッチャーを作成します。
func newMatcher(prog *program, input []rune) *Matcher {
	// キャプチャグループ用の配列を初期化
	// 各グループにつき2つの位置（開始と終了）が必要
	numSlots := (prog.numCaptures + 1) * 2
	saved := make([]int, numSlots)
	for i := range saved {
		saved[i] = -1 // 未初期化の位置は-1
	}

	// プログラムから情報を取得して設定
	var multiline, caseInsensitive, dotMatchesNL bool

	// プログラム内のフラグを確認
	for _, instr := range prog.instrs {
		// マルチラインモードを検出
		if instr.Op == InstrBeginLine || instr.Op == InstrEndLine {
			multiline = true
		}

		// 大文字小文字を区別しないモードを検出
		if instr.Op == InstrCharClass && instr.CharClass != nil && instr.CharClass.caseInsensitive {
			caseInsensitive = true
		}

		// ドットが改行にマッチするモードを検出
		if instr.Op == InstrAnyChar && instr.Arg == 1 {
			dotMatchesNL = true
		}
	}

	return &Matcher{
		prog:            prog,
		input:           input,
		pos:             0,
		multiline:       multiline,
		caseInsensitive: caseInsensitive,
		dotMatchesNL:    dotMatchesNL,
		startPos:        0,
		saved:           saved,
		maxSteps:        1000000, // 最大実行ステップ数（適宜調整）
	}
}

// Match は、入力文字列のどこかで正規表現がマッチするかどうかを確認します。
func (m *Matcher) Match() bool {
	// マルチラインモードが設定されているかどうかを確認
	if m.prog != nil && len(m.prog.instrs) > 0 {
		// プログラムの最初の命令にマルチラインフラグが設定されているか確認
		for _, instr := range m.prog.instrs {
			if instr.Op == InstrBeginLine || instr.Op == InstrEndLine {
				m.multiline = true
				break
			}
		}
	}

	// 入力の各位置からマッチングを試行
	for start := 0; start <= len(m.input); start++ {
		m.startPos = start
		m.pos = start
		// キャプチャ状態をリセット
		for i := range m.saved {
			m.saved[i] = -1
		}
		m.steps = 0

		// 最初のキャプチャグループ（全体マッチ）の開始位置を設定
		m.saved[0] = start

		// 命令列を実行
		if m.execute(0) {
			// マッチした場合、最初のキャプチャグループの終了位置を設定
			m.saved[1] = m.pos
			return true
		}
	}
	return false
}

// MatchStart は、入力文字列の指定位置から始まるマッチを確認します。
func (m *Matcher) MatchStart(start int) bool {
	if start < 0 || start > len(m.input) {
		return false
	}

	m.startPos = start
	m.pos = start
	// キャプチャ状態をリセット
	for i := range m.saved {
		m.saved[i] = -1
	}
	m.steps = 0

	// 最初のキャプチャグループ（全体マッチ）の開始位置を設定
	m.saved[0] = start

	// 命令列を実行
	if m.execute(0) {
		// マッチした場合、最初のキャプチャグループの終了位置を設定
		m.saved[1] = m.pos
		return true
	}
	return false
}

// Captures は、最後のマッチで捕捉されたグループの位置を返します。
func (m *Matcher) Captures() [][]int {
	result := make([][]int, (len(m.saved)+1)/2)
	for i := 0; i < len(result); i++ {
		start := m.saved[i*2]
		end := m.saved[i*2+1]
		if start >= 0 && end >= 0 {
			result[i] = []int{start, end}
		} else {
			result[i] = []int{-1, -1} // マッチしなかったグループ
		}
	}
	return result
}

// CaptureTexts は、最後のマッチで捕捉されたグループのテキストを返します。
func (m *Matcher) CaptureTexts() []string {
	caps := m.Captures()
	result := make([]string, len(caps))
	for i, cap := range caps {
		if cap[0] >= 0 && cap[1] >= 0 {
			result[i] = string(m.input[cap[0]:cap[1]])
		} else {
			result[i] = "" // マッチしなかったグループ
		}
	}
	return result
}

// execute は、命令列を実行します。
func (m *Matcher) execute(pc int) bool {
	// バックトラックスタック
	var stack []BacktrackPoint

	for {
		// 無限ループ防止
		m.steps++
		if m.steps > m.maxSteps {
			return false
		}

		// プログラムの終了チェック
		if pc >= len(m.prog.instrs) {
			return false
		}

		instr := m.prog.instrs[pc]

		switch instr.Op {
		case InstrMatch:
			// マッチ成功
			return true

		case InstrChar:
			// 1文字マッチ
			if m.pos >= len(m.input) {
				// 入力終了
				goto Backtrack
			}

			ch := m.input[m.pos]

			matched := false
			if m.caseInsensitive {
				// 大文字小文字を無視して比較
				matched = equalFoldRune(ch, instr.Char)
			} else {
				// 通常の比較
				matched = (ch == instr.Char)
			}

			if !matched {
				goto Backtrack
			}

			m.pos++
			pc = instr.Next

		case InstrAnyChar:
			// 任意の1文字マッチ
			if m.pos >= len(m.input) {
				// 入力終了
				goto Backtrack
			}

			ch := m.input[m.pos]
			// 改行にマッチするかどうか
			if !m.dotMatchesNL && (ch == '\n' || ch == '\r') {
				goto Backtrack
			}

			m.pos++
			pc = instr.Next

		case InstrCharClass:
			// 文字クラスマッチ
			if m.pos >= len(m.input) {
				// 入力終了
				goto Backtrack
			}

			ch := m.input[m.pos]
			if !instr.CharClass.matches(ch) {
				goto Backtrack
			}

			m.pos++
			pc = instr.Next

		case InstrJump:
			// 無条件ジャンプ
			pc = instr.Next

		case InstrSplit:
			// 条件分岐（バックトラックポイント）
			if instr.Possessive {
				// 所有的量指定子の場合、バックトラックしない
				if instr.Greedy {
					// 貪欲モード：最初の分岐を先に試す
					pc = instr.Next
				} else {
					// 非貪欲モード：2番目の分岐を先に試す
					pc = instr.Arg
				}
			} else {
				// 通常の分岐
				// バックトラックポイントをスタックに追加
				savepoint := make([]int, len(m.saved))
				copy(savepoint, m.saved)

				var nextPC, altPC int
				if instr.Greedy {
					// 貪欲モード：最初の分岐を先に試す
					nextPC = instr.Next
					altPC = instr.Arg
				} else {
					// 非貪欲モード：2番目の分岐を先に試す
					nextPC = instr.Arg
					altPC = instr.Next
				}

				// バックトラックポイントを保存
				stack = append(stack, BacktrackPoint{
					pc:       altPC,
					pos:      m.pos,
					captures: savepoint,
				})

				pc = nextPC
			}

		case InstrSave:
			// キャプチャグループの位置を保存
			slot := instr.Arg
			// 現在の位置を保存
			m.saved[slot] = m.pos
			pc = instr.Next

		case InstrBackref:
			// バックリファレンス
			groupIdx := instr.Arg
			startSlot := groupIdx * 2
			endSlot := startSlot + 1

			// 参照するグループがまだマッチしていない場合は失敗
			if startSlot >= len(m.saved) || m.saved[startSlot] < 0 || m.saved[endSlot] < 0 {
				goto Backtrack
			}

			startPos := m.saved[startSlot]
			endPos := m.saved[endSlot]
			refLen := endPos - startPos

			// 入力の残りが短い場合は失敗
			if m.pos+refLen > len(m.input) {
				goto Backtrack
			}

			// 参照したテキストと入力を比較
			for i := 0; i < refLen; i++ {
				if m.input[startPos+i] != m.input[m.pos+i] {
					goto Backtrack
				}
			}

			m.pos += refLen
			pc = instr.Next

		case InstrWordBoundary:
			// 単語境界
			atBoundary := isAtWordBoundary(m.input, m.pos)
			if !atBoundary {
				goto Backtrack
			}
			pc = instr.Next

		case InstrNonWordBoundary:
			// 非単語境界
			atBoundary := isAtWordBoundary(m.input, m.pos)
			if atBoundary {
				goto Backtrack
			}
			pc = instr.Next

		case InstrBeginLine:
			// 行頭
			if m.pos > 0 && m.input[m.pos-1] != '\n' && m.input[m.pos-1] != '\r' && (m.pos != m.startPos || !m.multiline) {
				goto Backtrack
			}
			pc = instr.Next

		case InstrEndLine:
			// 行末
			if m.pos < len(m.input) && m.input[m.pos] != '\n' && m.input[m.pos] != '\r' {
				goto Backtrack
			}
			pc = instr.Next

		case InstrBeginText:
			// テキスト先頭
			if m.pos != 0 {
				goto Backtrack
			}
			pc = instr.Next

		case InstrEndText:
			// テキスト末尾
			if m.pos != len(m.input) {
				goto Backtrack
			}
			pc = instr.Next

		default:
			// 未知の命令
			return false
		}

		continue

	Backtrack:
		// バックトラックポイントがあれば、そこから再開
		if len(stack) > 0 {
			bp := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			pc = bp.pc
			m.pos = bp.pos
			copy(m.saved, bp.captures)
		} else {
			// バックトラックポイントがなければ失敗
			return false
		}
	}
}

// isAtWordBoundary は、指定された位置が単語境界かどうかを判定します。
func isAtWordBoundary(input []rune, pos int) bool {
	left := false
	if pos > 0 {
		left = isWordChar(input[pos-1])
	}

	right := false
	if pos < len(input) {
		right = isWordChar(input[pos])
	}

	// 一方が単語文字で、もう一方が非単語文字の場合、境界
	return left != right
}

// matchString は、文字列に対してマッチングを行います。
func matchString(prog *program, s string) bool {
	runes := []rune(s)
	m := newMatcher(prog, runes)
	return m.Match()
}

// matchBytes は、バイト列に対してマッチングを行います。
func matchBytes(prog *program, b []byte) bool {
	s := string(b)
	return matchString(prog, s)
}

// matchReader は、Readerから読み取ったテキストに対してマッチングを行います。
func matchReader(prog *program, r io.RuneReader) bool {
	var runes []rune
	for {
		r, size, err := r.ReadRune()
		if err != nil {
			break
		}
		if size > 0 {
			runes = append(runes, r)
		}
	}
	m := newMatcher(prog, runes)
	return m.Match()
}

// findStringSubmatchIndex は、文字列内のマッチと各サブマッチの位置を返します。
func findStringSubmatchIndex(prog *program, s string) []int {
	runes := []rune(s)

	// 各位置からマッチを試行
	for start := 0; start <= len(runes); start++ {
		m := newMatcher(prog, runes)
		if m.MatchStart(start) {
			// マッチした場合、キャプチャグループの位置を返す
			caps := m.Captures()
			result := make([]int, len(caps)*2)
			for i, cap := range caps {
				if cap[0] >= 0 && cap[1] >= 0 {
					// ルーンインデックスからバイト位置に変換
					startBytes := runeSliceIndex(s, cap[0])
					endBytes := runeSliceIndex(s, cap[1])
					result[i*2] = startBytes
					result[i*2+1] = endBytes
				} else {
					result[i*2] = -1
					result[i*2+1] = -1
				}
			}
			return result
		}
	}

	return nil
}

// findStringSubmatch は、文字列内のマッチと各サブマッチのテキストを返します。
func findStringSubmatch(prog *program, s string) []string {
	runes := []rune(s)

	// 各位置からマッチを試行
	for start := 0; start <= len(runes); start++ {
		m := newMatcher(prog, runes)
		if m.MatchStart(start) {
			return m.CaptureTexts()
		}
	}

	return nil
}

// findSubmatch は、バイト列内のマッチと各サブマッチを返します。
func findSubmatch(prog *program, b []byte) [][]byte {
	matches := findStringSubmatch(prog, string(b))
	if matches == nil {
		return nil
	}

	result := make([][]byte, len(matches))
	for i, match := range matches {
		if match != "" {
			result[i] = []byte(match)
		}
	}
	return result
}

// findString は、文字列内の最初のマッチを返します。
func findString(prog *program, s string) string {
	matches := findStringSubmatch(prog, s)
	if matches == nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// find は、バイト列内の最初のマッチを返します。
func find(prog *program, b []byte) []byte {
	s := findString(prog, string(b))
	if s == "" {
		return nil
	}
	return []byte(s)
}

// findStringIndex は、文字列内のマッチの位置を返します。
func findStringIndex(prog *program, s string) []int {
	// 各位置からマッチを試行
	runes := []rune(s)
	for start := 0; start <= len(runes); start++ {
		m := newMatcher(prog, runes)
		if m.MatchStart(start) {
			// マッチした場合、開始位置と終了位置を返す
			caps := m.Captures()
			if len(caps) > 0 && caps[0][0] >= 0 && caps[0][1] >= 0 {
				// ルーンインデックスからバイト位置に変換
				startIdx := runeSliceIndex(s, caps[0][0])
				endIdx := runeSliceIndex(s, caps[0][1])
				return []int{startIdx, endIdx}
			}
		}
	}
	return nil
}

// findIndex は、バイト列内のマッチの位置を返します。
func findIndex(prog *program, b []byte) []int {
	return findStringIndex(prog, string(b))
}

// runeSliceIndex は、文字列内のルーンインデックスに対応するバイトインデックスを返します。
func runeSliceIndex(s string, runeIdx int) int {
	if runeIdx <= 0 {
		return 0
	}

	// rune単位のインデックスをバイト単位のインデックスに変換
	count := 0
	for i := range s {
		if count == runeIdx {
			return i
		}
		count++
	}
	return len(s)
}

// equalFoldRune は、2つのruneが大文字小文字を区別せずに等しいかどうかを判定します。
func equalFoldRune(r1, r2 rune) bool {
	// 同じ文字ならtrue
	if r1 == r2 {
		return true
	}

	// 小文字に変換して比較
	r1Lower := toLowerRune(r1)
	r2Lower := toLowerRune(r2)
	return r1Lower == r2Lower
}

// toLowerRune は、runeを小文字に変換します。ライブラリ未使用の実装
func toLowerRune(r rune) rune {
	// ASCII範囲の大文字の場合
	if 'A' <= r && r <= 'Z' {
		return r + ('a' - 'A')
	}

	// 以下は簡略化した実装です。完全なUnicode対応にはunicodeパッケージが必要
	// ここでは英語のアルファベットのみサポート
	return r
}

// トランスパイラの警告：isWordChar関数はcompiler.goで定義されているため、
// ここでの定義は削除します。
