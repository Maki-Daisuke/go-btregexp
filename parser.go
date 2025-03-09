// Package btregexp は、バックトラック型の正規表現エンジンを実装したパッケージです。
package btregexp

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// Parser は、正規表現の構文解析を行うパーサーです。
type Parser struct {
	input       string         // 解析する正規表現パターン
	pos         int            // 現在の解析位置
	width       int            // 最後に読んだ文字のサイズ
	captures    int            // キャプチャグループの数
	capNames    map[string]int // 名前付きキャプチャグループ名と番号のマッピング
	subexpNames []string       // キャプチャグループの名前のリスト
	flags       regexpFlags    // 現在有効なフラグ
}

// regexpFlags は、正規表現のフラグを表します。
type regexpFlags struct {
	caseInsensitive bool // 大小文字を区別しない (?i)
	multiline       bool // マルチラインモード (?m)
	dotMatchesNL    bool // . が改行にもマッチする (?s)
	ungreedy        bool // デフォルトで非貪欲 (?U)
}

// newParser は、新しいパーサーを作成します。
func newParser(input string) *Parser {
	return &Parser{
		input:       input,
		capNames:    make(map[string]int),
		subexpNames: []string{""}, // インデックス0は常に空文字列（マッチ全体用）
	}
}

// Parse は、正規表現パターンを解析して抽象構文木を構築します。
// また、パース中に検出したフラグも返します。
func (p *Parser) Parse() (Node, regexpFlags, error) {
	// パターン全体の解析を開始
	expr, err := p.parseExpr()
	if err != nil {
		return nil, p.flags, err
	}

	// すべての入力が消費されたか確認
	if p.pos < len(p.input) {
		return nil, p.flags, fmt.Errorf("予期しない文字: %q", p.peek())
	}

	return expr, p.flags, nil
}

// parseExpr は、トップレベルの式（正規表現の全体）をパースします。
// 内部では選択演算子（|）を処理します。
func (p *Parser) parseExpr() (Node, error) {
	// 最初のサブ式を解析
	left, err := p.parseConcat()
	if err != nil {
		return nil, err
	}

	// 選択演算子（|）が続く場合、右側のサブ式も解析
	for p.peek() == '|' {
		p.next() // '|' を消費
		right, err := p.parseConcat()
		if err != nil {
			return nil, err
		}
		// 左辺と右辺を選択ノードに組み立て
		left = &AltNode{left: left, right: right}
	}

	return left, nil
}

// parseConcat は、連接を解析します（例: "ab"）。
// 連続する項目を連接ノードとして処理します。
func (p *Parser) parseConcat() (Node, error) {
	var nodes []Node

	// 連接の各項を処理
	for {
		// 連接を終了する文字をチェック
		r := p.peek()
		if r == 0 || r == '|' || r == ')' {
			break
		}

		// 次の項を解析
		item, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, item)
	}

	// 連接ノードが1つしかない場合は、そのノードをそのまま返す
	if len(nodes) == 1 {
		return nodes[0], nil
	}

	// 複数のノードがある場合は、連接ノードを作成
	return &ConcatNode{nodes: nodes}, nil
}

// parseTerm は、繰り返し演算子（*, +, ?, {n,m}）を含む単一の項を解析します。
func (p *Parser) parseTerm() (Node, error) {
	// 基本的な要素（文字、グループなど）を解析
	atom, err := p.parseAtom()
	if err != nil {
		return nil, err
	}

	// 繰り返し演算子が続くかチェック
	switch p.peek() {
	case '*', '+', '?':
		return p.parseRepeat(atom)
	case '{':
		if pos := p.pos + 1; pos < len(p.input) {
			// {n,m} 形式の範囲指定量指定子をチェック
			if isDigit(rune(p.input[pos])) {
				return p.parseRepeatRange(atom)
			}
		}
	}

	return atom, nil
}

// parseRepeat は、*, +, ? の繰り返し演算子を解析します。
func (p *Parser) parseRepeat(node Node) (Node, error) {
	r := p.next() // 繰り返し演算子を消費

	// 繰り返しのタイプを決定
	var min, max int
	switch r {
	case '*':
		min, max = 0, -1 // -1 は無限大を意味する
	case '+':
		min, max = 1, -1
	case '?':
		min, max = 0, 1
	default:
		return nil, fmt.Errorf("無効な繰り返し演算子: %c", r)
	}

	// 貪欲さを決定（デフォルトは貪欲、? が続けば非貪欲）
	repeatType := RepeatGreedy
	if p.peek() == '?' {
		p.next() // ? を消費
		repeatType = RepeatNonGreedy
	} else if p.peek() == '+' {
		p.next() // + を消費
		// 所有的量指定子 (*+, ++, ?+) の実装
		return &RepeatNode{
			node:       node,
			min:        min,
			max:        max,
			repeatType: RepeatGreedy,
			possessive: true,
		}, nil
	}

	// エンジン設定で非貪欲モードが指定されている場合、貪欲さを反転
	if p.flags.ungreedy {
		if repeatType == RepeatGreedy {
			repeatType = RepeatNonGreedy
		} else {
			repeatType = RepeatGreedy
		}
	}

	return &RepeatNode{
		node:       node,
		min:        min,
		max:        max,
		repeatType: repeatType,
	}, nil
}

// parseRepeatRange は、{n,m} 形式の範囲指定繰り返しを解析します。
func (p *Parser) parseRepeatRange(node Node) (Node, error) {
	p.next() // '{' を消費

	// 最小繰り返し回数を解析
	min, err := p.parseNumber()
	if err != nil {
		return nil, err
	}

	max := min // デフォルトでは min と同じ値
	if p.peek() == ',' {
		p.next() // ',' を消費
		if p.peek() == '}' {
			max = -1 // {n,} 形式（上限なし）
		} else {
			// 最大繰り返し回数を解析
			max, err = p.parseNumber()
			if err != nil {
				return nil, err
			}
		}
	}

	// 閉じ括弧を確認
	if p.peek() != '}' {
		return nil, fmt.Errorf("閉じ括弧 '}' がありません: %s", p.input[p.pos:])
	}
	p.next() // '}' を消費

	// 貪欲さを決定（デフォルトは貪欲、? が続けば非貪欲）
	repeatType := RepeatGreedy
	possessive := false

	if p.peek() == '?' {
		p.next() // ? を消費
		repeatType = RepeatNonGreedy
	} else if p.peek() == '+' {
		p.next() // + を消費
		possessive = true
	}

	// エンジン設定で非貪欲モードが指定されている場合、貪欲さを反転
	if p.flags.ungreedy {
		if repeatType == RepeatGreedy {
			repeatType = RepeatNonGreedy
		} else {
			repeatType = RepeatGreedy
		}
	}

	return &RepeatNode{
		node:       node,
		min:        min,
		max:        max,
		repeatType: repeatType,
		possessive: possessive,
	}, nil
}

// parseNumber は、繰り返し回数などの数値を解析します。
func (p *Parser) parseNumber() (int, error) {
	start := p.pos
	for isDigit(p.peek()) {
		p.next()
	}

	if start == p.pos {
		return 0, fmt.Errorf("数値が必要です: %s", p.input[p.pos:])
	}

	n, err := strconv.Atoi(p.input[start:p.pos])
	if err != nil {
		return 0, fmt.Errorf("無効な数値: %s", p.input[start:p.pos])
	}

	return n, nil
}

// parseAtom は、基本的な正規表現要素（文字、文字クラス、グループなど）を解析します。
func (p *Parser) parseAtom() (Node, error) {
	r := p.peek()

	switch r {
	case 0:
		return nil, fmt.Errorf("予期しない入力終了")
	case '|', '*', '+', '?', '}':
		return nil, fmt.Errorf("予期しない文字: %c", r)
	case '.':
		p.next() // '.' を消費
		return &AnyCharNode{dotMatchesNewline: p.flags.dotMatchesNL}, nil
	case '[':
		return p.parseCharClass()
	case '(':
		return p.parseGroup()
	case ')':
		return nil, fmt.Errorf("閉じ括弧に対応する開き括弧がありません")
	case '\\':
		return p.parseEscape()
	case '^':
		p.next() // '^' を消費
		return &BoundaryNode{nodeType: NodeBeginLine}, nil
	case '$':
		p.next() // '$' を消費
		return &BoundaryNode{nodeType: NodeEndLine}, nil
	default:
		p.next() // 文字を消費
		return &CharNode{r: r}, nil
	}
}

// parseGroup は、括弧で囲まれたグループを解析します。
func (p *Parser) parseGroup() (Node, error) {
	p.next() // '(' を消費

	// グループタイプをチェック
	if p.peek() == '?' {
		p.next() // '?' を消費
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("グループの設定が不完全です")
		}

		// グループタイプを処理
		switch p.peek() {
		case ':':
			// 非キャプチャグループ (?:...)
			p.next() // ':' を消費
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if p.peek() != ')' {
				return nil, fmt.Errorf("閉じ括弧 ')' がありません")
			}
			p.next() // ')' を消費
			return &GroupNode{node: expr}, nil

		case 'P':
			// 名前付きキャプチャグループ (?P<name>...)
			return p.parseNamedCapture()

		case 'i', 'm', 's', 'U':
			// フラグ設定 (?i), (?m), (?s), (?U)
			return p.parseFlags()

		default:
			return nil, fmt.Errorf("不明なグループ指定: %c", p.peek())
		}
	}

	// 通常のキャプチャグループ
	p.captures++
	index := p.captures
	p.subexpNames = append(p.subexpNames, "")

	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if p.peek() != ')' {
		return nil, fmt.Errorf("閉じ括弧 ')' がありません")
	}
	p.next() // ')' を消費

	return &CaptureNode{
		index: index,
		node:  expr,
	}, nil
}

// parseNamedCapture は、名前付きキャプチャグループ (?P<name>...) を解析します。
func (p *Parser) parseNamedCapture() (Node, error) {
	// "P<" を確認
	if p.next() != 'P' || p.next() != '<' {
		return nil, fmt.Errorf("無効な名前付きキャプチャグループ形式: (?P")
	}

	// グループ名を解析
	start := p.pos
	for {
		r := p.peek()
		if r == 0 {
			return nil, fmt.Errorf("閉じ括弧 '>' がありません")
		}
		if r == '>' {
			break
		}
		p.next()
	}

	name := p.input[start:p.pos]
	if name == "" {
		return nil, fmt.Errorf("名前付きキャプチャグループに名前がありません")
	}

	p.next() // '>' を消費

	// 名前が既に使用されているかチェック
	if _, exists := p.capNames[name]; exists {
		return nil, fmt.Errorf("キャプチャグループ名が重複しています: %s", name)
	}

	// キャプチャグループを登録
	p.captures++
	index := p.captures
	p.capNames[name] = index
	p.subexpNames = append(p.subexpNames, name)

	// グループの内容を解析
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if p.peek() != ')' {
		return nil, fmt.Errorf("閉じ括弧 ')' がありません")
	}
	p.next() // ')' を消費

	return &CaptureNode{
		index: index,
		name:  name,
		node:  expr,
	}, nil
}

// parseFlags は、正規表現のフラグを解析します。
func (p *Parser) parseFlags() (Node, error) {
	// フラグを読み取る
	oldFlags := p.flags
	onFlags, offFlags := p.parseModifiers()

	// フラグを適用
	if onFlags.caseInsensitive {
		p.flags.caseInsensitive = true
	}
	if offFlags.caseInsensitive {
		p.flags.caseInsensitive = false
	}

	if onFlags.multiline {
		p.flags.multiline = true
	}
	if offFlags.multiline {
		p.flags.multiline = false
	}

	if onFlags.dotMatchesNL {
		p.flags.dotMatchesNL = true
	}
	if offFlags.dotMatchesNL {
		p.flags.dotMatchesNL = false
	}

	if onFlags.ungreedy {
		p.flags.ungreedy = true
	}
	if offFlags.ungreedy {
		p.flags.ungreedy = false
	}

	// グループがある場合（(?i:...)）
	if p.peek() == ':' {
		p.next() // ':' を消費

		// グループの内容を解析
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}

		if p.peek() != ')' {
			return nil, fmt.Errorf("閉じ括弧 ')' がありません")
		}
		p.next() // ')' を消費

		// フラグを元に戻す（フラグの効果はこのグループ内だけ）
		p.flags = oldFlags

		return expr, nil
	}

	// グループがない場合（(?i)）
	if p.peek() != ')' {
		return nil, fmt.Errorf("閉じ括弧 ')' がありません")
	}
	p.next() // ')' を消費

	// 空のグループを返す（フラグ設定のみのグループは内容がない）
	return &GroupNode{node: &ConcatNode{nodes: []Node{}}}, nil
}

// parseModifiers は、フラグ修飾子を解析します。
func (p *Parser) parseModifiers() (onFlags, offFlags regexpFlags) {
	for {
		switch p.peek() {
		case 'i':
			p.next()
			onFlags.caseInsensitive = true
		case 'm':
			p.next()
			onFlags.multiline = true
		case 's':
			p.next()
			onFlags.dotMatchesNL = true
		case 'U':
			p.next()
			onFlags.ungreedy = true
		case '-':
			// 負のフラグ（無効化）の開始
			p.next()
			// 負のフラグを解析
			for {
				switch p.peek() {
				case 'i':
					p.next()
					offFlags.caseInsensitive = true
				case 'm':
					p.next()
					offFlags.multiline = true
				case 's':
					p.next()
					offFlags.dotMatchesNL = true
				case 'U':
					p.next()
					offFlags.ungreedy = true
				default:
					return
				}
			}
		default:
			return
		}
	}
}

// parseCharClass は、文字クラス（[...]）を解析します。
func (p *Parser) parseCharClass() (Node, error) {
	p.next() // '[' を消費

	// 否定クラスかどうかチェック
	negate := false
	if p.peek() == '^' {
		negate = true
		p.next() // '^' を消費
	}

	node := &CharClassNode{
		classType: ClassCustom,
		negate:    negate,
	}

	// 文字クラスの内容を解析
	for p.peek() != ']' && p.peek() != 0 {
		min, err := p.parseClassAtom()
		if err != nil {
			return nil, err
		}

		// 範囲があるかチェック（[a-z]）
		max := min
		if p.peek() == '-' {
			p.next() // '-' を消費
			if p.peek() == ']' {
				// ハイフンが文字クラスの最後にある場合、リテラルとして扱う
				node.ranges = append(node.ranges, runeRange{min: min, max: min})
				node.ranges = append(node.ranges, runeRange{min: '-', max: '-'})
				continue
			}

			// 範囲の上限を解析
			max, err = p.parseClassAtom()
			if err != nil {
				return nil, err
			}

			if max < min {
				return nil, fmt.Errorf("無効な文字範囲: %c-%c", min, max)
			}
		}

		node.ranges = append(node.ranges, runeRange{min: min, max: max})
	}

	if p.peek() != ']' {
		return nil, fmt.Errorf("閉じ括弧 ']' がありません")
	}
	p.next() // ']' を消費

	return node, nil
}

// parseClassAtom は、文字クラス内の1文字またはエスケープシーケンスを解析します。
func (p *Parser) parseClassAtom() (rune, error) {
	r := p.peek()

	if r == 0 {
		return 0, fmt.Errorf("予期しない入力終了")
	}

	if r == '\\' {
		p.next() // '\\' を消費
		esc := p.peek()
		if esc == 0 {
			return 0, fmt.Errorf("予期しない入力終了")
		}
		p.next() // エスケープ文字を消費

		// 特殊文字のエスケープを処理
		switch esc {
		case 'n':
			return '\n', nil
		case 'r':
			return '\r', nil
		case 't':
			return '\t', nil
		case 'f':
			return '\f', nil
		case 'v':
			return '\v', nil
		default:
			// それ以外はそのまま返す（\., \*, \[ など）
			return esc, nil
		}
	}

	// 通常の文字
	p.next() // 文字を消費
	return r, nil
}

// parseEscape は、バックスラッシュでエスケープされた文字を解析します。
func (p *Parser) parseEscape() (Node, error) {
	p.next() // '\\' を消費

	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("エスケープシーケンスが終了していません")
	}

	r := p.peek()
	p.next() // エスケープ文字を消費

	switch r {
	// メタ文字のエスケープ
	case '.', '*', '+', '?', '|', '(', ')', '[', ']', '{', '}', '\\', '^', '$':
		return &CharNode{r: r}, nil

	// よく使われるエスケープシーケンス
	case 'n':
		return &CharNode{r: '\n'}, nil
	case 'r':
		return &CharNode{r: '\r'}, nil
	case 't':
		return &CharNode{r: '\t'}, nil
	case 'f':
		return &CharNode{r: '\f'}, nil
	case 'v':
		return &CharNode{r: '\v'}, nil

	// 文字クラスのショートカット
	case 'd':
		return &CharClassNode{classType: ClassDigit}, nil
	case 'D':
		return &CharClassNode{classType: ClassDigit, negate: true}, nil
	case 'w':
		return &CharClassNode{classType: ClassWord}, nil
	case 'W':
		return &CharClassNode{classType: ClassWord, negate: true}, nil
	case 's':
		return &CharClassNode{classType: ClassSpace}, nil
	case 'S':
		return &CharClassNode{classType: ClassSpace, negate: true}, nil

	// アンカー
	case 'A':
		return &BoundaryNode{nodeType: NodeBeginText}, nil
	case 'z':
		return &BoundaryNode{nodeType: NodeEndText}, nil
	case 'b':
		return &BoundaryNode{nodeType: NodeWordBoundary}, nil
	case 'B':
		return &BoundaryNode{nodeType: NodeNonWordBoundary}, nil

	// バックリファレンス
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		index := int(r - '0')
		if index > p.captures {
			return nil, fmt.Errorf("存在しないキャプチャグループへの参照: \\%d", index)
		}
		return &BackrefNode{index: index}, nil

	// Unicodeプロパティ
	case 'p', 'P':
		isNegative := r == 'P'

		if p.peek() != '{' {
			return nil, fmt.Errorf("Unicodeプロパティは \\p{...} 形式でなければなりません")
		}
		p.next() // '{' を消費

		start := p.pos
		for p.peek() != '}' && p.peek() != 0 {
			p.next()
		}

		if p.peek() != '}' {
			return nil, fmt.Errorf("閉じ括弧 '}' がありません")
		}

		propertyName := p.input[start:p.pos]
		p.next() // '}' を消費

		return &CharClassNode{
			classType:  ClassUnicode,
			negate:     isNegative,
			unicodeKey: propertyName,
		}, nil

	// 名前付きバックリファレンス
	case 'k':
		if p.peek() != '<' {
			return nil, fmt.Errorf("名前付きバックリファレンスは \\k<name> 形式でなければなりません")
		}
		p.next() // '<' を消費

		start := p.pos
		for p.peek() != '>' && p.peek() != 0 {
			p.next()
		}

		if p.peek() != '>' {
			return nil, fmt.Errorf("閉じ括弧 '>' がありません")
		}

		name := p.input[start:p.pos]
		p.next() // '>' を消費

		index, ok := p.capNames[name]
		if !ok {
			return nil, fmt.Errorf("存在しない名前付きキャプチャグループへの参照: \\k<%s>", name)
		}

		return &BackrefNode{index: index, name: name}, nil

	default:
		// その他のエスケープは単なる文字として扱う
		return &CharNode{r: r}, nil
	}
}

// next は入力から次の文字を取得し、位置を進めます。
func (p *Parser) next() rune {
	if p.pos >= len(p.input) {
		p.width = 0
		return 0
	}

	r, width := utf8.DecodeRuneInString(p.input[p.pos:])
	p.width = width
	p.pos += p.width
	return r
}

// peek は、次の文字を取得しますが、位置は進めません。
func (p *Parser) peek() rune {
	if p.pos >= len(p.input) {
		return 0
	}

	r, _ := utf8.DecodeRuneInString(p.input[p.pos:])
	return r
}

// isDigit は、rが数字かどうかを返します。
func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

// Flags は、コンパイル時に使用するフラグを表します。
type Flags struct {
	CaseInsensitive bool // 大小文字を区別しない
	Multiline       bool // マルチラインモード
	DotMatchesNL    bool // ドットが改行にもマッチ
	Ungreedy        bool // デフォルトで非貪欲
}
