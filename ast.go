// Package btregexp は、バックトラック型の正規表現エンジンを実装したパッケージです。
package btregexp

// NodeType は、ASTノードの種類を表します。
type NodeType int

const (
	NodeUnknown         NodeType = iota
	NodeChar                     // 単一文字
	NodeConcat                   // 連接
	NodeAlt                      // 選択（|）
	NodeStar                     // 0回以上の繰り返し（*）
	NodePlus                     // 1回以上の繰り返し（+）
	NodeQuest                    // 0または1回（?）
	NodeRepeat                   // 範囲指定の繰り返し（{n,m}）
	NodeCapture                  // キャプチャグループ（括弧）
	NodeGroup                    // 非キャプチャグループ
	NodeBackref                  // バックリファレンス（\1, \2, ...）
	NodeAnyChar                  // 任意の1文字（.）
	NodeCharClass                // 文字クラス（[...]）
	NodeBeginLine                // 行頭（^）
	NodeEndLine                  // 行末（$）
	NodeBeginText                // テキスト先頭（\A）
	NodeEndText                  // テキスト末尾（\z）
	NodeWordBoundary             // 単語境界（\b）
	NodeNonWordBoundary          // 非単語境界（\B）
)

// RepeatType は、繰り返しの種類を表します。
type RepeatType int

const (
	RepeatGreedy    RepeatType = iota // 貪欲（最大マッチ）
	RepeatNonGreedy                   // 非貪欲（最小マッチ）
)

// CharClassType は、文字クラスの種類を表します。
type CharClassType int

const (
	ClassCustom  CharClassType = iota // カスタム文字クラス（[a-z]など）
	ClassDigit                        // 数字（\d）
	ClassWord                         // 単語文字（\w）
	ClassSpace                        // 空白文字（\s）
	ClassUnicode                      // Unicodeプロパティ（\p{...}）
)

// runeRange は、文字クラスでの文字範囲を表します。
type runeRange struct {
	min rune // 範囲の最小値
	max rune // 範囲の最大値
}

// Node は、正規表現の抽象構文木のノードを表すインターフェースです。
type Node interface {
	Type() NodeType
}

// CharNode は、単一の文字にマッチするノードです。
type CharNode struct {
	r rune // マッチする文字
}

func (n *CharNode) Type() NodeType {
	return NodeChar
}

// ConcatNode は、複数のノードの連接を表します。
type ConcatNode struct {
	nodes []Node // 連接されるノードのリスト
}

func (n *ConcatNode) Type() NodeType {
	return NodeConcat
}

// AltNode は、選択（|）を表します。
type AltNode struct {
	left  Node // 左辺
	right Node // 右辺
}

func (n *AltNode) Type() NodeType {
	return NodeAlt
}

// RepeatNode は、繰り返し（*, +, ?, {n,m}）を表します。
type RepeatNode struct {
	node       Node       // 繰り返される部分
	min        int        // 最小繰り返し回数
	max        int        // 最大繰り返し回数（-1は無限大）
	repeatType RepeatType // 貪欲または非貪欲
	possessive bool       // 所有的量指定子かどうか（*+, ++, ?+）
}

func (n *RepeatNode) Type() NodeType {
	if n.min == 0 && n.max == -1 {
		return NodeStar // *
	} else if n.min == 1 && n.max == -1 {
		return NodePlus // +
	} else if n.min == 0 && n.max == 1 {
		return NodeQuest // ?
	} else {
		return NodeRepeat // {n,m}
	}
}

// CaptureNode は、キャプチャグループを表します。
type CaptureNode struct {
	index int    // キャプチャグループのインデックス
	name  string // キャプチャグループの名前（名前付きキャプチャの場合）
	node  Node   // グループの内容
}

func (n *CaptureNode) Type() NodeType {
	return NodeCapture
}

// GroupNode は、非キャプチャグループを表します。
type GroupNode struct {
	node Node // グループの内容
}

func (n *GroupNode) Type() NodeType {
	return NodeGroup
}

// BackrefNode は、バックリファレンスを表します。
type BackrefNode struct {
	index int    // 参照するキャプチャグループのインデックス
	name  string // 参照するキャプチャグループの名前（名前参照の場合）
}

func (n *BackrefNode) Type() NodeType {
	return NodeBackref
}

// AnyCharNode は、任意の1文字（.）にマッチするノードです。
type AnyCharNode struct {
	dotMatchesNewline bool // 改行にもマッチするかどうか
}

func (n *AnyCharNode) Type() NodeType {
	return NodeAnyChar
}

// CharClassNode は、文字クラス（[...]）を表します。
type CharClassNode struct {
	classType  CharClassType // 文字クラスの種類
	negate     bool          // 否定クラスかどうか（[^...]）
	ranges     []runeRange   // 文字範囲のリスト（カスタムクラスの場合）
	unicodeKey string        // Unicodeプロパティ（\p{...}の場合）
}

func (n *CharClassNode) Type() NodeType {
	return NodeCharClass
}

// BoundaryNode は、各種境界条件（^, $, \b, \B, \A, \z）を表します。
type BoundaryNode struct {
	nodeType NodeType // 境界の種類
}

func (n *BoundaryNode) Type() NodeType {
	return n.nodeType
}
