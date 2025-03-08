# バックトラック型正規表現エンジン仕様

## 概要
このプロジェクトは、Go言語でバックトラック型の正規表現エンジンを実装するものです。
標準ライブラリのregexpパッケージと互換性のあるAPIを提供しつつ、内部実装はバックトラッキングアルゴリズムを採用します。

## アーキテクチャ

エンジンは主に以下の3つのコンポーネントで構成されます：

1. **構文解析器（Parser）**：
   - 正規表現パターン文字列を解析し、内部表現（抽象構文木）に変換
   - フラグ処理（(?i), (?m)など）
   - キャプチャグループの管理

2. **コンパイラ（Compiler）**：
   - 構文木を実行可能な形式（命令列）に変換
   - 最適化（可能な場合）

3. **マッチングエンジン（Matcher）**：
   - バックトラッキングアルゴリズムによるマッチング処理
   - 状態管理（現在位置、バックトラック情報）
   - キャプチャグループの値の記録

## サポートする機能

### 基本的な文字マッチング
- リテラル文字（a, b, c, ...）
- メタ文字（., \d, \w, \s, ...）
- 文字クラス（[a-z], [^0-9], ...）
- Unicode対応（\p{Greek}, \P{Punctuation}, ...）

### 量指定子
- 0回以上（*）
- 1回以上（+）
- 0または1回（?）
- 範囲指定（{n}, {n,}, {n,m}）
- 非貪欲マッチング（*?, +?, ??, {n,m}?）

### グループと参照
- キャプチャグループ（(...)）
- 非キャプチャグループ（(?:...)）
- 名前付きキャプチャグループ（(?P<name>...)）
- バックリファレンス（\1, \2, ..., \k<name>）

### アンカーと境界
- 行頭（^）、行末（$）
- テキスト先頭（\A）、テキスト末尾（\z）
- 単語境界（\b）、非単語境界（\B）

### フラグ
- 大小文字を区別しない（(?i)）
- マルチラインモード（(?m)）
- ドットが改行にもマッチ（(?s)）
- 非貪欲モード（(?U)）

## API

標準ライブラリのregexpパッケージと互換性のあるAPIを提供します。主なメソッド：

### コンパイル関数
```go
func Compile(expr string) (*Regexp, error)
func MustCompile(expr string) *Regexp
```

### マッチング関数
```go
func (re *Regexp) Match(b []byte) bool
func (re *Regexp) MatchString(s string) bool
func (re *Regexp) MatchReader(r io.RuneReader) bool
```

### 検索関数
```go
func (re *Regexp) Find(b []byte) []byte
func (re *Regexp) FindString(s string) string
func (re *Regexp) FindIndex(b []byte) []int
func (re *Regexp) FindStringIndex(s string) []int
```

### サブマッチ関数
```go
func (re *Regexp) FindSubmatch(b []byte) [][]byte
func (re *Regexp) FindStringSubmatch(s string) []string
func (re *Regexp) FindSubmatchIndex(b []byte) []int
func (re *Regexp) FindStringSubmatchIndex(s string) []int
```

### その他の関数
```go
func (re *Regexp) NumSubexp() int
func (re *Regexp) SubexpNames() []string
func (re *Regexp) String() string
```

## バックトラッキングアルゴリズム

1. **基本アプローチ**：
   - 深さ優先探索（DFS）による再帰的なマッチング
   - 失敗したら前の選択点に戻り、別の選択肢を試行

2. **状態管理**：
   - 現在の入力位置
   - 現在のパターン位置
   - キャプチャグループの状態
   - バックトラックポイント（選択的分岐点）

3. **最適化**：
   - 先読み/先行否定 の実装
   - バックトラックポイントの効率的な管理
   - 単純なパターンの早期検出と最適化

## 実装計画

1. **基本構造**：
   - Regexp型と基本的なAPIの宣言
   - 内部データ構造の設計

2. **構文解析器**：
   - 正規表現パターンの解析
   - 抽象構文木の構築

3. **コンパイラ**：
   - 命令セットの設計
   - 構文木から命令列への変換

4. **マッチングエンジン**：
   - バックトラッキングアルゴリズムの実装
   - キャプチャグループのサポート

5. **機能拡張**：
   - Unicode対応
   - 応用的な機能の追加

## 性能とリミテーション

- バックトラック型のため、特定のパターンで指数関数的な実行時間になる可能性があります
- 過度に複雑なバックトラッキングを防ぐために、実行ステップ数に上限を設けることも検討します
- 標準ライブラリのregexpに比べて、一部のケースでは低速になる可能性があります

## 今後の拡張可能性

- より高度なパターンマッチング機能
- パフォーマンス最適化
- Look-ahead / Look-behind のサポート
- Atomic grouping のサポート
- Possessive quantifiers のサポート
