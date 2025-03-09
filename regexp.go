// Package btregexp は、バックトラック型の正規表現エンジンを実装したパッケージです。
// このパッケージは、Go標準ライブラリのregexpパッケージと互換性のあるAPIを提供します。
package btregexp

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"
)

// Regexp は、コンパイルされた正規表現を表します。
// この構造体はスレッドセーフであり、複数のゴルーチンから同時に使用できます。
type Regexp struct {
	// 正規表現のソースパターン
	expr string

	// コンパイルされた正規表現プログラム
	prog *program

	// 正規表現が持つサブマッチ（キャプチャグループ）の数
	numSubexp int

	// サブマッチの名前（名前付きキャプチャグループ用）
	subexpNames []string
}

// program は、コンパイルされた正規表現プログラムを表します。
type program struct {
	// 命令列
	instrs []Instr

	// キャプチャグループの数
	numCaptures int

	// サブマッチの名前のリスト
	subexpNames []string
}

// CompileWithFlags は、フラグを指定して正規表現パターンをコンパイルします。
func CompileWithFlags(expr string, flags Flags) (*Regexp, error) {
	// パーサーを作成
	parser := newParser(expr)

	// パーサーのフラグを設定
	parser.flags = regexpFlags{
		caseInsensitive: flags.CaseInsensitive,
		multiline:       flags.Multiline,
		dotMatchesNL:    flags.DotMatchesNL,
		ungreedy:        flags.Ungreedy,
	}

	// 正規表現をパース
	ast, parsedFlags, err := parser.Parse()
	if err != nil {
		return nil, err
	}

	// コンパイラーを作成
	compiler := newCompiler()

	// フラグをマージ
	mergedFlags := Flags{
		CaseInsensitive: flags.CaseInsensitive || parsedFlags.caseInsensitive,
		Multiline:       flags.Multiline || parsedFlags.multiline,
		DotMatchesNL:    flags.DotMatchesNL || parsedFlags.dotMatchesNL,
		Ungreedy:        flags.Ungreedy || parsedFlags.ungreedy,
	}
	compiler.flags = mergedFlags

	// ASTをコンパイル
	prog, err := compiler.compile(ast)
	if err != nil {
		return nil, err
	}

	// Regexpオブジェクトを作成
	re := &Regexp{
		expr:        expr,
		prog:        prog,
		numSubexp:   compiler.numCaptures,
		subexpNames: compiler.subexpNames,
	}

	return re, nil
}

// Compile は、正規表現パターンをコンパイルし、Regexpオブジェクトを返します。
// パターンが無効な場合はエラーを返します。
//
// 正規表現内で使用できるフラグ修飾子:
// (?i) - 大小文字を区別しない
// (?m) - マルチラインモード: ^ と $ が各行の先頭と末尾にマッチ
// (?s) - . が改行を含む任意の文字にマッチ
// (?U) - デフォルトで非貪欲マッチング (*, +, ? などが最小マッチに)
func Compile(expr string) (*Regexp, error) {
	return compile(expr)
}

// MustCompile は Compile と同様ですが、コンパイルに失敗した場合はパニックします。
func MustCompile(expr string) *Regexp {
	re, err := Compile(expr)
	if err != nil {
		panic("regexp: Compile(" + quote(expr) + "): " + err.Error())
	}
	return re
}

// compile は、実際のコンパイル処理を行う内部関数です。
func compile(expr string) (*Regexp, error) {
	// デフォルトフラグでコンパイル
	return CompileWithFlags(expr, Flags{})
}

// Quote は、文字列内の特殊文字をエスケープします。
func Quote(s string) string {
	if !strings.ContainsAny(s, ".$^{[(|)*+?\\") {
		return s
	}

	var sb strings.Builder
	for _, c := range s {
		switch c {
		case '.', '$', '^', '{', '[', '(', '|', ')', '*', '+', '?', '\\':
			sb.WriteByte('\\')
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

// quote は内部的にデバッグログに使うエスケープ関数です。
func quote(s string) string {
	if len(s) > 100 {
		return s[:97] + "..."
	}
	return s
}

// Match は、bのどこかで正規表現がマッチするかどうかを報告します。
func (re *Regexp) Match(b []byte) bool {
	return matchBytes(re.prog, b)
}

// MatchString は、sのどこかで正規表現がマッチするかどうかを報告します。
func (re *Regexp) MatchString(s string) bool {
	return matchString(re.prog, s)
}

// MatchReader は、rから読み取ったテキストのどこかで正規表現がマッチするかどうかを報告します。
func (re *Regexp) MatchReader(r io.RuneReader) bool {
	return matchReader(re.prog, r)
}

// Find は、bの中で正規表現にマッチする最初の部分文字列を返します。
// マッチしない場合はnilを返します。
func (re *Regexp) Find(b []byte) []byte {
	return find(re.prog, b)
}

// FindString は、sの中で正規表現にマッチする最初の部分文字列を返します。
// マッチしない場合は空文字列を返します。
func (re *Regexp) FindString(s string) string {
	return findString(re.prog, s)
}

// FindIndex は、bの中で正規表現にマッチする最初の部分文字列の位置を返します。
// 戻り値のスライスには、マッチの開始位置と終了位置が含まれます。
// マッチしない場合はnilを返します。
func (re *Regexp) FindIndex(b []byte) []int {
	return findIndex(re.prog, b)
}

// FindStringIndex は、sの中で正規表現にマッチする最初の部分文字列の位置を返します。
// 戻り値のスライスには、マッチの開始位置と終了位置が含まれます。
// マッチしない場合はnilを返します。
func (re *Regexp) FindStringIndex(s string) []int {
	return findStringIndex(re.prog, s)
}

// FindSubmatch は、bの中で正規表現にマッチする最初の部分文字列と、
// 各サブマッチ（キャプチャグループ）を返します。
// 戻り値のスライスの最初の要素は、マッチ全体に対応します。
// マッチしない場合はnilを返します。
func (re *Regexp) FindSubmatch(b []byte) [][]byte {
	return findSubmatch(re.prog, b)
}

// FindStringSubmatch は、sの中で正規表現にマッチする最初の部分文字列と、
// 各サブマッチ（キャプチャグループ）を返します。
// 戻り値のスライスの最初の要素は、マッチ全体に対応します。
// マッチしない場合はnilを返します。
func (re *Regexp) FindStringSubmatch(s string) []string {
	return findStringSubmatch(re.prog, s)
}

// FindSubmatchIndex は、bの中で正規表現にマッチする最初の部分文字列と、
// 各サブマッチ（キャプチャグループ）の位置を返します。
// 戻り値のスライスには、マッチ全体の開始位置と終了位置、
// 続いて各サブマッチの開始位置と終了位置が含まれます。
// マッチしない場合はnilを返します。
func (re *Regexp) FindSubmatchIndex(b []byte) []int {
	return findStringSubmatchIndex(re.prog, string(b))
}

// FindStringSubmatchIndex は、sの中で正規表現にマッチする最初の部分文字列と、
// 各サブマッチ（キャプチャグループ）の位置を返します。
// 戻り値のスライスには、マッチ全体の開始位置と終了位置、
// 続いて各サブマッチの開始位置と終了位置が含まれます。
// マッチしない場合はnilを返します。
func (re *Regexp) FindStringSubmatchIndex(s string) []int {
	return findStringSubmatchIndex(re.prog, s)
}

// NumSubexp は、この正規表現内のサブマッチ（キャプチャグループ）の数を返します。
func (re *Regexp) NumSubexp() int {
	return re.numSubexp
}

// SubexpNames は、この正規表現内のサブマッチ（キャプチャグループ）の名前を返します。
// 最初の要素はマッチ全体を表し、常に空文字列です。
func (re *Regexp) SubexpNames() []string {
	return re.subexpNames
}

// Longest メソッドは標準ライブラリとの互換性のために存在しますが、
// 初版のバックトラック型エンジンでは実装していません。
func (re *Regexp) Longest() {
	// 初版では機能しません
}

// String は、この正規表現のソースパターンを返します。
func (re *Regexp) String() string {
	return re.expr
}

// ReplaceAll は、bの中でマッチする全ての部分文字列をrepl（の展開）で置き換えます。
// 展開では、$1, $2, ...はキャプチャグループの内容に置き換えられます。
// $0はマッチ全体に置き換えられます。
func (re *Regexp) ReplaceAll(src, repl []byte) []byte {
	return re.replaceAll(src, repl, false)
}

// ReplaceAllString は、sの中でマッチする全ての部分文字列をrepl（の展開）で置き換えます。
// 展開では、$1, $2, ...はキャプチャグループの内容に置き換えられます。
// $0はマッチ全体に置き換えられます。
func (re *Regexp) ReplaceAllString(src, repl string) string {
	return string(re.replaceAll([]byte(src), []byte(repl), false))
}

// ReplaceAllLiteralString は、マッチする全ての部分文字列をreplで置き換えます（展開なし）。
func (re *Regexp) ReplaceAllLiteralString(src, repl string) string {
	return string(re.replaceAll([]byte(src), []byte(repl), true))
}

// replaceAll は、すべての置換を処理する内部関数です。
func (re *Regexp) replaceAll(src, repl []byte, literal bool) []byte {
	// マッチを検索
	indices := re.FindSubmatchIndex(src)
	if indices == nil {
		return src // マッチしなければ元の文字列を返す
	}

	var result bytes.Buffer
	lastEnd := 0

	// マッチごとに処理
	for indices != nil {
		// マッチ前の部分を追加
		result.Write(src[lastEnd:indices[0]])

		// 置換テキストを処理
		if literal {
			// リテラル置換
			result.Write(repl)
		} else {
			// 展開付き置換
			expanded := re.expandReplacement(repl, src, indices)
			result.Write(expanded)
		}

		// 次の検索開始位置を更新
		lastEnd = indices[1]

		// 次のマッチを検索
		if lastEnd >= len(src) {
			break
		}
		indices = re.FindSubmatchIndex(src[lastEnd:])
		if indices != nil {
			// 検索開始位置を調整
			for i := range indices {
				indices[i] += lastEnd
			}
		}
	}

	// 最後のマッチ以降の部分を追加
	if lastEnd < len(src) {
		result.Write(src[lastEnd:])
	}

	return result.Bytes()
}

// expandReplacement は、置換テキスト内の$1, $2, ...を展開します。
func (re *Regexp) expandReplacement(repl, src []byte, indices []int) []byte {
	var result bytes.Buffer
	for i := 0; i < len(repl); i++ {
		if repl[i] == '$' && i+1 < len(repl) {
			i++ // $の次の文字へ
			switch {
			case repl[i] == '$':
				// $$は$にエスケープ
				result.WriteByte('$')
			case '0' <= repl[i] && repl[i] <= '9':
				// グループ参照
				group := int(repl[i] - '0')
				// 2桁の数字も扱う
				if i+1 < len(repl) && '0' <= repl[i+1] && repl[i+1] <= '9' {
					group = group*10 + int(repl[i+1]-'0')
					if group <= re.numSubexp {
						i++
					} else {
						// 2桁目が有効なグループでない場合は1桁目だけ
						group = int(repl[i] - '0')
					}
				}
				// グループが有効な範囲かチェック
				if group <= re.numSubexp && 2*group+1 < len(indices) {
					start, end := indices[2*group], indices[2*group+1]
					if start >= 0 && end >= 0 {
						result.Write(src[start:end])
					}
				}
			default:
				// 不明な$シーケンスは$そのものとして処理
				result.WriteByte('$')
				result.WriteByte(repl[i])
			}
		} else {
			// 通常の文字
			result.WriteByte(repl[i])
		}
	}
	return result.Bytes()
}

// FindAllStringSubmatch は、sの中で正規表現にマッチするすべての部分文字列と、
// 各サブマッチ（キャプチャグループ）を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAllStringSubmatch(s string, n int) [][]string {
	if n == 0 {
		return nil
	}

	var result [][]string
	start := 0

	for {
		if n > 0 && len(result) >= n {
			break
		}

		// 現在位置からマッチを検索
		input := s[start:]
		runes := []rune(input)
		m := newMatcher(re.prog, runes)
		if !m.Match() {
			break
		}

		// マッチ結果を取得
		caps := m.CaptureTexts()
		result = append(result, caps)

		// マッチの終了位置を取得（次の検索開始位置）
		matchPos := m.Captures()[0]
		if matchPos[0] == matchPos[1] {
			// 空マッチの場合は1文字進める
			start += 1
		} else {
			// 通常のマッチの場合はマッチの終了位置から
			runeEnd := matchPos[1]
			if runeEnd > 0 {
				// ルーンインデックスからバイトインデックスへ変換
				byteEnd := runeSliceIndex(input, runeEnd)
				start += byteEnd
			}
		}

		// 検索文字列の終わりに達した場合は終了
		if start >= len(s) {
			break
		}
	}

	return result
}

// Split は、正規表現がマッチする位置で文字列を分割します。
// nが正の場合は最大でn個の部分文字列を返し、それ以外の場合はすべての部分文字列を返します。
func (re *Regexp) Split(s string, n int) []string {
	if n == 0 {
		return nil
	}

	if n < 0 {
		n = len(s) + 1
	}

	var result []string
	matches := re.FindAllStringIndex(s, n-1)

	if matches == nil {
		return []string{s}
	}

	lastEnd := 0
	for _, match := range matches {
		// マッチ前の部分を結果に追加
		result = append(result, s[lastEnd:match[0]])
		lastEnd = match[1]
	}

	// 最後のマッチ以降の部分を追加
	result = append(result, s[lastEnd:])

	return result
}

// FindAllStringIndex は、sの中で正規表現にマッチするすべての部分文字列の位置を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAllStringIndex(s string, n int) [][]int {
	if n == 0 {
		return nil
	}

	var result [][]int
	start := 0

	for {
		if n > 0 && len(result) >= n {
			break
		}

		// 現在位置からマッチを検索
		input := s[start:]
		index := re.FindStringIndex(input)
		if index == nil {
			break
		}

		// マッチ位置を調整（全体の文字列での位置に）
		adjustedIndex := []int{start + index[0], start + index[1]}
		result = append(result, adjustedIndex)

		// 次の検索開始位置を更新
		start = adjustedIndex[1]
		if start == adjustedIndex[0] {
			// 空マッチの場合は1文字進める
			if start < len(s) {
				_, size := utf8.DecodeRuneInString(s[start:])
				start += size
			} else {
				break
			}
		}

		// 検索文字列の終わりに達した場合は終了
		if start >= len(s) {
			break
		}
	}

	return result
}

// FindAllSubmatch は、bの中で正規表現にマッチするすべての部分文字列と、
// 各サブマッチ（キャプチャグループ）を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAllSubmatch(b []byte, n int) [][][]byte {
	matches := re.FindAllStringSubmatch(string(b), n)
	if matches == nil {
		return nil
	}

	result := make([][][]byte, len(matches))
	for i, match := range matches {
		byteMatch := make([][]byte, len(match))
		for j, s := range match {
			if s != "" {
				byteMatch[j] = []byte(s)
			}
		}
		result[i] = byteMatch
	}

	return result
}

// FindAll は、bの中で正規表現にマッチするすべての部分文字列を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAll(b []byte, n int) [][]byte {
	matches := re.FindAllStringSubmatch(string(b), n)
	if matches == nil {
		return nil
	}

	result := make([][]byte, len(matches))
	for i, match := range matches {
		if len(match) > 0 && match[0] != "" {
			result[i] = []byte(match[0])
		}
	}

	return result
}

// FindAllString は、sの中で正規表現にマッチするすべての部分文字列を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAllString(s string, n int) []string {
	matches := re.FindAllStringSubmatch(s, n)
	if matches == nil {
		return nil
	}

	result := make([]string, len(matches))
	for i, match := range matches {
		if len(match) > 0 {
			result[i] = match[0]
		}
	}

	return result
}

// FindAllSubmatchIndex は、bの中で正規表現にマッチするすべての部分文字列と、
// 各サブマッチ（キャプチャグループ）の位置を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAllSubmatchIndex(b []byte, n int) [][]int {
	return re.FindAllStringSubmatchIndex(string(b), n)
}

// FindAllStringSubmatchIndex は、sの中で正規表現にマッチするすべての部分文字列と、
// 各サブマッチ（キャプチャグループ）の位置を返します。
// nが負の場合はすべてのマッチを返し、それ以外の場合は最大でn個のマッチを返します。
func (re *Regexp) FindAllStringSubmatchIndex(s string, n int) [][]int {
	if n == 0 {
		return nil
	}

	var result [][]int
	start := 0

	for {
		if n > 0 && len(result) >= n {
			break
		}

		// 現在位置からマッチを検索
		input := s[start:]
		indices := re.FindStringSubmatchIndex(input)
		if indices == nil {
			break
		}

		// マッチ位置を調整（全体の文字列での位置に）
		for i := range indices {
			if indices[i] >= 0 {
				indices[i] += start
			}
		}
		result = append(result, indices)

		// 次の検索開始位置を更新
		matchEnd := indices[1]
		if matchEnd == indices[0] {
			// 空マッチの場合は1文字進める
			if start < len(s) {
				_, size := utf8.DecodeRuneInString(s[start:])
				start += size
			} else {
				break
			}
		} else {
			start = matchEnd
		}

		// 検索文字列の終わりに達した場合は終了
		if start >= len(s) {
			break
		}
	}

	return result
}
