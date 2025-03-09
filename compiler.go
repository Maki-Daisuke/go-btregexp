// Package btregexp は、バックトラック型の正規表現エンジンを実装したパッケージです。
package btregexp

import (
	"fmt"
	"unicode"
)

// InstrType は、正規表現命令のタイプを表します。
type InstrType int

const (
	// 基本命令
	InstrChar            InstrType = iota // 文字とマッチ
	InstrAnyChar                          // 任意の1文字とマッチ（.）
	InstrCharClass                        // 文字クラスとマッチ
	InstrMatch                            // マッチ成功
	InstrJump                             // 無条件ジャンプ
	InstrSplit                            // 分岐（バックトラック用）
	InstrSave                             // キャプチャグループの開始・終了位置を保存
	InstrBackref                          // バックリファレンス
	InstrWordBoundary                     // 単語境界
	InstrNonWordBoundary                  // 非単語境界
	InstrBeginLine                        // 行頭
	InstrEndLine                          // 行末
	InstrBeginText                        // テキスト先頭
	InstrEndText                          // テキスト末尾
)

// SaveType は、InstrSaveのタイプを表します。
type SaveType int

const (
	SaveBegin SaveType = iota // グループの開始位置を保存
	SaveEnd                   // グループの終了位置を保存
)

// Instr は、正規表現プログラムの命令を表します。
type Instr struct {
	Op         InstrType  // 命令の種類
	Next       int        // 次の命令インデックス（-1は終了、分岐命令では第2分岐先）
	Arg        int        // 命令の引数（文字、ジャンプ先、保存位置など）
	SaveType   SaveType   // InstrSaveの場合、開始位置か終了位置か
	Char       rune       // InstrCharの場合の文字
	CharClass  *charClass // InstrCharClassの場合の文字クラス
	Greedy     bool       // InstrSplitの場合、貪欲マッチか非貪欲マッチか
	Possessive bool       // 所有的量指定子か
}

// charClass は、文字クラスの内部表現です。
type charClass struct {
	anyOf           []rune          // 含まれる個別の文字
	ranges          []runeRange     // 含まれる文字範囲
	classType       CharClassType   // 組み込み文字クラス（\d, \s, \w など）
	negate          bool            // 否定文字クラスかどうか（[^...] など）
	unicode         map[string]bool // Unicodeプロパティ
	caseInsensitive bool            // 大小文字を区別しないかどうか
}

// matches は、文字 r が文字クラスにマッチするかどうかを判定します。
func (c *charClass) matches(r rune) bool {
	// 個別の文字をチェック
	for _, ch := range c.anyOf {
		if r == ch || (c.caseInsensitive && unicode.ToLower(r) == unicode.ToLower(ch)) {
			return !c.negate
		}
	}

	// 文字範囲をチェック
	for _, rng := range c.ranges {
		if r >= rng.min && r <= rng.max {
			return !c.negate
		}
		// 大小文字を区別しない場合、小文字変換してから再チェック
		if c.caseInsensitive {
			lowerR := unicode.ToLower(r)
			if lowerR >= unicode.ToLower(rng.min) && lowerR <= unicode.ToLower(rng.max) {
				return !c.negate
			}
		}
	}

	// 組み込み文字クラスをチェック
	switch c.classType {
	case ClassDigit:
		if '0' <= r && r <= '9' {
			return !c.negate
		}
	case ClassWord:
		if isWordChar(r) {
			return !c.negate
		}
	case ClassSpace:
		if unicode.IsSpace(r) {
			return !c.negate
		}
	case ClassUnicode:
		// TODO: Unicodeプロパティの実装
		// （現在はダミー実装です）
		for prop := range c.unicode {
			if prop == "L" && unicode.IsLetter(r) {
				return !c.negate
			}
		}
	}

	// デフォルトでは否定の逆
	return c.negate
}

// isWordChar は、文字が単語構成文字（\w）かどうかを判定します。
func isWordChar(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') || r == '_'
}

// Compiler は、正規表現のコンパイラを表します。
type Compiler struct {
	instrs      []Instr  // 生成される命令列
	numCaptures int      // キャプチャグループの数
	subexpNames []string // サブマッチの名前のリスト
	flags       Flags    // コンパイル時のフラグ
}

// newCompiler は、新しいコンパイラを作成します。
func newCompiler() *Compiler {
	return &Compiler{
		instrs:      make([]Instr, 0, 16), // 十分な初期容量を確保
		numCaptures: 0,
	}
}

// emit は、命令をプログラムに追加します。
func (c *Compiler) emit(instr Instr) int {
	pos := len(c.instrs)
	c.instrs = append(c.instrs, instr)
	return pos
}

// patch は、指定された位置の命令のNext（またはArg）を更新します。
func (c *Compiler) patch(pos int, next int) {
	c.instrs[pos].Next = next
}

// patchArg は、指定された位置の命令のArgを更新します。
func (c *Compiler) patchArg(pos int, arg int) {
	c.instrs[pos].Arg = arg
}

// compile は、ASTノードをコンパイルして命令列を生成します。
func (c *Compiler) compile(node Node) (*program, error) {
	// ルートノードからコンパイル開始
	start, err := c.compileNode(node)
	if err != nil {
		return nil, err
	}

	// マッチング成功命令を追加
	c.emit(Instr{Op: InstrMatch})

	// 命令列の先頭を調整
	if start != 0 {
		// 命令列の先頭が0以外の場合（通常起こりえない）
		newInstrs := make([]Instr, len(c.instrs))
		copy(newInstrs, c.instrs)
		// 先頭を0番目に移動
		for i := range newInstrs {
			if newInstrs[i].Op == InstrJump || newInstrs[i].Op == InstrSplit {
				if newInstrs[i].Next >= start {
					newInstrs[i].Next -= start
				}
				if newInstrs[i].Arg >= 0 && newInstrs[i].Arg >= start {
					newInstrs[i].Arg -= start
				}
			}
		}
		c.instrs = newInstrs[:len(c.instrs)-start]
	}

	// 完成したプログラムを返す
	return &program{
		instrs:      c.instrs,
		numCaptures: c.numCaptures,
		subexpNames: c.subexpNames,
	}, nil
}

// compileNode は、指定されたノードとその子ノードをコンパイルします。
func (c *Compiler) compileNode(node Node) (int, error) {
	if node == nil {
		return -1, fmt.Errorf("ノードがnilです")
	}

	switch n := node.(type) {
	case *CharNode:
		// 1文字にマッチする命令を生成
		char := n.r
		if c.flags.CaseInsensitive {
			char = unicode.ToLower(char)
		}
		start := c.emit(Instr{Op: InstrChar, Char: char, Next: len(c.instrs) + 1})
		return start, nil

	case *AnyCharNode:
		// 任意の1文字にマッチする命令を生成
		dotMatchesNL := n.dotMatchesNewline || c.flags.DotMatchesNL
		start := c.emit(Instr{
			Op:   InstrAnyChar,
			Arg:  boolToInt(dotMatchesNL), // 1なら改行にもマッチ、0ならマッチしない
			Next: len(c.instrs) + 1,
		})
		return start, nil

	case *CharClassNode:
		// 文字クラスにマッチする命令を生成
		class := &charClass{
			classType:       n.classType,
			negate:          n.negate,
			caseInsensitive: c.flags.CaseInsensitive,
		}

		// カスタム文字クラスの場合、範囲をコピー
		if n.classType == ClassCustom {
			for _, r := range n.ranges {
				class.ranges = append(class.ranges, r)
			}
		} else if n.classType == ClassUnicode {
			// Unicodeプロパティの場合
			class.unicode = make(map[string]bool)
			class.unicode[n.unicodeKey] = true
		}

		start := c.emit(Instr{
			Op:        InstrCharClass,
			CharClass: class,
			Next:      len(c.instrs) + 1,
		})
		return start, nil

	case *ConcatNode:
		// 連接は、各ノードを順番にコンパイル
		if len(n.nodes) == 0 {
			// 空の連接は空文字にマッチ（ジャンプだけ）
			return c.emit(Instr{Op: InstrJump, Next: len(c.instrs) + 1}), nil
		}

		var start int
		var err error
		var lastNext int

		for i, child := range n.nodes {
			if i == 0 {
				start, err = c.compileNode(child)
				if err != nil {
					return -1, err
				}
				lastNext = len(c.instrs)
			} else {
				curr, err := c.compileNode(child)
				if err != nil {
					return -1, err
				}
				c.patch(lastNext-1, curr)
				lastNext = len(c.instrs)
			}
		}

		return start, nil

	case *AltNode:
		// 選択（|）は、分岐命令を使用
		// 左辺と右辺をコンパイル
		left, err := c.compileNode(n.left)
		if err != nil {
			return -1, err
		}

		// 左辺が終了したら終点にジャンプする命令を追加
		jumpPos := c.emit(Instr{Op: InstrJump, Next: -1}) // 後でパッチ

		_, err = c.compileNode(n.right)
		if err != nil {
			return -1, err
		}
		rightEnd := len(c.instrs)

		// 左辺のジャンプ先を、右辺の終了後に設定
		c.patch(jumpPos, rightEnd)

		// 分岐命令を先頭に挿入し、左辺か右辺にジャンプするようにする
		start := c.emit(Instr{Op: InstrSplit, Next: left, Arg: jumpPos + 1})

		return start, nil

	case *RepeatNode:
		// 繰り返しノードは複雑なので、タイプ別に処理
		switch n.Type() {
		case NodeStar: // 0回以上の繰り返し(*)
			return c.compileStar(n.node, n.repeatType == RepeatNonGreedy, n.possessive)
		case NodePlus: // 1回以上の繰り返し(+)
			return c.compilePlus(n.node, n.repeatType == RepeatNonGreedy, n.possessive)
		case NodeQuest: // 0または1回(?)
			return c.compileQuest(n.node, n.repeatType == RepeatNonGreedy, n.possessive)
		case NodeRepeat: // 範囲指定({n,m})
			return c.compileRepeat(n.node, n.min, n.max, n.repeatType == RepeatNonGreedy, n.possessive)
		}
		return -1, fmt.Errorf("未知の繰り返しタイプ: %v", n.Type())

	case *CaptureNode:
		// キャプチャグループ
		// 開始位置を保存する命令
		captureIndex := n.index * 2 // 開始と終了で2つの位置を保存
		saveBegin := c.emit(Instr{
			Op:       InstrSave,
			Arg:      captureIndex,
			SaveType: SaveBegin,
			Next:     len(c.instrs) + 1,
		})

		// グループの内容をコンパイル
		body, err := c.compileNode(n.node)
		if err != nil {
			return -1, err
		}

		// 最初の保存命令の次の命令を内容の先頭に設定
		c.patch(saveBegin, body)

		// 終了位置を保存する命令
		c.emit(Instr{
			Op:       InstrSave,
			Arg:      captureIndex + 1,
			SaveType: SaveEnd,
			Next:     len(c.instrs) + 1,
		})

		// 必要なら、キャプチャ情報を更新
		if n.index > c.numCaptures {
			c.numCaptures = n.index
		}

		// サブマッチ名のリストを初期化する必要がある場合
		if len(c.subexpNames) == 0 {
			c.subexpNames = make([]string, 1) // インデックス0は空文字列（マッチ全体）
		}

		// キャプチャインデックスに対応する名前を設定
		for len(c.subexpNames) <= n.index {
			c.subexpNames = append(c.subexpNames, "")
		}
		if n.name != "" {
			c.subexpNames[n.index] = n.name
		}

		return saveBegin, nil

	case *GroupNode:
		// 非キャプチャグループは単純に内容をコンパイル
		return c.compileNode(n.node)

	case *BackrefNode:
		// バックリファレンス
		var refIndex int
		if n.name != "" {
			// 名前付きバックリファレンスの場合、番号を検索する必要がある
			// （ここでは単純化のため、インデックスが直接指定されたと仮定）
			refIndex = n.index
		} else {
			refIndex = n.index
		}

		// バックリファレンス命令を生成
		start := c.emit(Instr{
			Op:   InstrBackref,
			Arg:  refIndex,
			Next: len(c.instrs) + 1,
		})
		return start, nil

	case *BoundaryNode:
		// 境界条件
		var op InstrType
		switch n.nodeType {
		case NodeWordBoundary:
			op = InstrWordBoundary
		case NodeNonWordBoundary:
			op = InstrNonWordBoundary
		case NodeBeginLine:
			op = InstrBeginLine
		case NodeEndLine:
			op = InstrEndLine
		case NodeBeginText:
			op = InstrBeginText
		case NodeEndText:
			op = InstrEndText
		default:
			return -1, fmt.Errorf("未知の境界タイプ: %v", n.nodeType)
		}

		start := c.emit(Instr{
			Op:   op,
			Next: len(c.instrs) + 1,
		})
		return start, nil

	default:
		return -1, fmt.Errorf("未知のノードタイプ: %T", node)
	}
}

// compileStar は、0回以上の繰り返し（*）をコンパイルします。
func (c *Compiler) compileStar(node Node, nonGreedy, possessive bool) (int, error) {
	// 先に分岐命令を挿入（後で本体の先頭を設定）
	var splitPos int

	if possessive {
		// 所有的量指定子の場合、単体にマッチした後は戻らない
		body, err := c.compileNode(node)
		if err != nil {
			return -1, err
		}

		// 先頭に分岐を置き、マッチするか、スキップするか
		splitPos = c.emit(Instr{
			Op:         InstrSplit,
			Next:       body, // マッチを試行
			Arg:        -1,   // スキップ（後でパッチ）
			Greedy:     !nonGreedy,
			Possessive: true,
		})

		// 本体の最後に、本体の先頭に戻るジャンプを追加（バックトラックなし）
		c.emit(Instr{
			Op:   InstrJump,
			Next: body,
		})

		// 分岐命令のスキップ先をここに設定
		c.patchArg(splitPos, len(c.instrs))
	} else {
		// 通常の*演算子
		splitPos = c.emit(Instr{
			Op:     InstrSplit,
			Next:   -1, // 後でパッチ
			Arg:    -1, // 後でパッチ
			Greedy: !nonGreedy,
		})

		// 本体をコンパイル
		body, err := c.compileNode(node)
		if err != nil {
			return -1, err
		}

		// 分岐命令の分岐先を本体に設定
		if nonGreedy {
			// 非貪欲の場合、先にスキップを試す
			c.patch(splitPos, len(c.instrs)+1) // スキップ
			c.patchArg(splitPos, body)         // マッチ
		} else {
			// 貪欲の場合、先にマッチを試す
			c.patch(splitPos, body)               // マッチ
			c.patchArg(splitPos, len(c.instrs)+1) // スキップ
		}

		// 本体の後に、繰り返し先頭に戻るジャンプを追加
		c.emit(Instr{
			Op:   InstrJump,
			Next: splitPos,
		})
	}

	return splitPos, nil
}

// compilePlus は、1回以上の繰り返し（+）をコンパイルします。
func (c *Compiler) compilePlus(node Node, nonGreedy, possessive bool) (int, error) {
	// まず、本体をコンパイル（これは最低1回実行）
	start, err := c.compileNode(node)
	if err != nil {
		return -1, err
	}

	if possessive {
		// 所有的量指定子の場合、最初にマッチした後は繰り返すがバックトラックはしない
		c.emit(Instr{
			Op:   InstrJump,
			Next: start,
		})
		return start, nil
	}

	// 次に分岐命令を挿入（本体に戻るか、次に進むか）
	var splitInstr Instr
	if nonGreedy {
		// 非貪欲の場合、先にスキップ
		splitInstr = Instr{
			Op:     InstrSplit,
			Next:   len(c.instrs) + 2, // スキップ
			Arg:    start,             // 繰り返し
			Greedy: false,
		}
	} else {
		// 貪欲の場合、先に繰り返し
		splitInstr = Instr{
			Op:     InstrSplit,
			Next:   start,             // 繰り返し
			Arg:    len(c.instrs) + 2, // スキップ
			Greedy: true,
		}
	}

	c.emit(splitInstr)

	return start, nil
}

// compileQuest は、0または1回（?）をコンパイルします。
func (c *Compiler) compileQuest(node Node, nonGreedy, possessive bool) (int, error) {
	// 分岐命令を挿入（本体を実行するか、スキップするか）
	splitPos := c.emit(Instr{
		Op:         InstrSplit,
		Next:       -1, // 後でパッチ
		Arg:        -1, // 後でパッチ
		Greedy:     !nonGreedy,
		Possessive: possessive,
	})

	// 本体をコンパイル
	body, err := c.compileNode(node)
	if err != nil {
		return -1, err
	}

	// 所有的量指定子の場合、バックトラック情報は破棄される

	// 分岐命令の分岐先を設定
	if nonGreedy {
		// 非貪欲の場合、先にスキップを試す
		c.patch(splitPos, len(c.instrs)+1) // スキップ
		c.patchArg(splitPos, body)         // マッチ
	} else {
		// 貪欲の場合、先にマッチを試す
		c.patch(splitPos, body)               // マッチ
		c.patchArg(splitPos, len(c.instrs)+1) // スキップ
	}

	return splitPos, nil
}

// compileRepeat は、範囲指定繰り返し（{n,m}）をコンパイルします。
func (c *Compiler) compileRepeat(node Node, min, max int, nonGreedy, possessive bool) (int, error) {
	// まず、最小回数分、本体を繰り返す
	var start, prev, current int
	var err error

	// 最小回数の繰り返し部分（固定実行）
	if min > 0 {
		// 最初の一回
		start, err = c.compileNode(node)
		if err != nil {
			return -1, err
		}

		prev = start

		// 残りの min-1 回
		for i := 1; i < min; i++ {
			current, err = c.compileNode(node)
			if err != nil {
				return -1, err
			}

			// 前のイテレーションと連結
			last := prev
			for c.instrs[last].Next != -1 && last < len(c.instrs)-1 {
				last++
			}
			if last < len(c.instrs) {
				c.patch(last, current)
			}

			prev = current
		}
	} else {
		// min == 0 の場合は、空のノードから始める
		start = len(c.instrs)
	}

	// 最大回数まで（オプショナルな追加実行）
	if max == -1 {
		// 上限なしの場合は * と同様
		// 最小回数を実行した後の位置
		if possessive {
			// 所有的量指定子の場合、マッチするがバックトラックしない
			repeatBody, err := c.compileNode(node)
			if err != nil {
				return -1, err
			}

			// 分岐：マッチするか終了するか
			splitPos := c.emit(Instr{
				Op:         InstrSplit,
				Next:       repeatBody,
				Arg:        len(c.instrs) + 2, // 終了位置（後でパッチ）
				Greedy:     !nonGreedy,
				Possessive: true,
			})

			// 繰り返しジャンプ
			c.emit(Instr{
				Op:   InstrJump,
				Next: splitPos,
			})
		} else {
			// 通常の繰り返し
			repeatBody, err := c.compileNode(node)
			if err != nil {
				return -1, err
			}

			// 分岐：マッチするか終了するか
			var splitOp Instr
			if nonGreedy {
				splitOp = Instr{
					Op:     InstrSplit,
					Next:   len(c.instrs) + 2, // 終了位置
					Arg:    repeatBody,        // マッチ
					Greedy: false,
				}
			} else {
				splitOp = Instr{
					Op:     InstrSplit,
					Next:   repeatBody,        // マッチ
					Arg:    len(c.instrs) + 2, // 終了位置
					Greedy: true,
				}
			}
			splitPos := c.emit(splitOp)

			// 繰り返しジャンプ
			c.emit(Instr{
				Op:   InstrJump,
				Next: splitPos,
			})
		}
	} else if max > min {
		// 有限の追加繰り返し
		var repeatStarts []int

		// 各追加の繰り返しで、実行するかスキップするかの分岐を追加
		for i := 0; i < max-min; i++ {
			// ここから繰り返し部分が始まる
			repeatPos := len(c.instrs)
			repeatStarts = append(repeatStarts, repeatPos)

			// 分岐命令：実行するかスキップするか
			splitPos := c.emit(Instr{
				Op:         InstrSplit,
				Next:       -1, // 後でパッチ
				Arg:        -1, // 後でパッチ
				Greedy:     !nonGreedy,
				Possessive: possessive,
			})

			// 本体をコンパイル
			body, err := c.compileNode(node)
			if err != nil {
				return -1, err
			}

			// 分岐命令の分岐先を設定
			if nonGreedy {
				// 非貪欲の場合、先にスキップを試す
				c.patch(splitPos, len(c.instrs)) // スキップ（次の分岐またはマッチ終了）
				c.patchArg(splitPos, body)       // マッチ
			} else {
				// 貪欲の場合、先にマッチを試す
				c.patch(splitPos, body)             // マッチ
				c.patchArg(splitPos, len(c.instrs)) // スキップ（次の分岐またはマッチ終了）
			}

			// 所有的量指定子の場合、バックトラック状態を破棄
			if possessive && i < max-min-1 {
				// 最後以外の繰り返しでは、次の繰り返しに無条件ジャンプ
				c.emit(Instr{
					Op:   InstrJump,
					Next: len(c.instrs),
				})
			}
		}
	}

	// 最終的には min 回目の先頭か、min == 0 の場合は最初の分岐を返す
	return start, nil
}

// boolToInt は、論理値を整数に変換します。
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
