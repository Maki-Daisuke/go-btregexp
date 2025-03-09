# Go-BTRegex - バックトラック型正規表現エンジン

Go-BTRegexは、Goで実装されたバックトラック型の正規表現エンジンです。標準のGo `regexp`パッケージはRE2に基づいた線形時間の正規表現エンジンですが、Go-BTRegexはバックトラッキングを使用してより高度なパターンマッチング機能を提供します。

## 特徴

- 標準のGoの`regexp`パッケージと互換性のあるAPI
- バックトラック方式のマッチング
- キャプチャグループ（名前付きキャプチャグループを含む）
- バックリファレンス
- 貪欲/非貪欲な量指定子
- 所有的量指定子（possessive quantifiers）
- 正規表現フラグ
  - `(?i)` - 大小文字を区別しない
  - `(?m)` - マルチライン
  - `(?s)` - ドットが改行にもマッチ
  - `(?U)` - デフォルトで非貪欲

## 使用方法

### 基本的な使い方

```go
package main

import (
    "fmt"
    "github.com/yourusername/go-btregex"
)

func main() {
    // 正規表現をコンパイル
    re, err := btregexp.Compile("a(b+)c")
    if err != nil {
        fmt.Println("コンパイルエラー:", err)
        return
    }

    // マッチしているかどうかをチェック
    if re.MatchString("abbbc") {
        fmt.Println("マッチしました")
    }

    // サブマッチ（キャプチャグループ）を取得
    match := re.FindStringSubmatch("abbbc")
    if match != nil {
        fmt.Println("全体のマッチ:", match[0])  // abbbc
        fmt.Println("グループ1のマッチ:", match[1])  // bbb
    }

    // テキスト内のすべてのマッチを置換
    result := re.ReplaceAllString("ac abc abbc", "X")
    fmt.Println(result)  // ac X X
}
```

### 高度な使用例

#### バックリファレンス

```go
re := btregexp.MustCompile(`(a+)b\1`)
fmt.Println(re.MatchString("aabaa"))  // true
fmt.Println(re.MatchString("abba"))   // false
```

#### 名前付きキャプチャグループ

```go
re := btregexp.MustCompile(`(?P<prefix>a+)b(?P<suffix>c+)`)
match := re.FindStringSubmatch("aabccc")
names := re.SubexpNames()

for i, name := range names {
    if i > 0 && name != "" {
        fmt.Printf("%s: %s\n", name, match[i])
    }
}
```

#### フラグの使用

```go
// 大小文字を区別しないマッチング
re := btregexp.MustCompile(`(?i)abc`)
fmt.Println(re.MatchString("ABC"))  // true

// ドットが改行にもマッチ
re = btregexp.MustCompile(`(?s)a.c`)
fmt.Println(re.MatchString("a\nc"))  // true

// フラグをグループに制限
re = btregexp.MustCompile(`(?i:a)bc`)
fmt.Println(re.MatchString("Abc"))  // true
fmt.Println(re.MatchString("ABC"))  // false
```

#### 所有的量指定子

```go
// 通常の繰り返し（バックトラックあり）
re1 := btregexp.MustCompile(`a+b`)

// 所有的量指定子（バックトラックなし）
re2 := btregexp.MustCompile(`a++b`)

fmt.Println(re1.MatchString("aaab"))  // true
fmt.Println(re2.MatchString("aaab"))  // true

// 次のパターンでは通常の+はバックトラックするが、++はしない
fmt.Println(re1.MatchString("aaabc"))  // true
fmt.Println(re2.MatchString("aaabc"))  // false
```

## 設計と実装

Go-BTRegexは、次の主要なコンポーネントで構成されています：

1. **パーサー** - 正規表現パターンを構文解析し、抽象構文木（AST）を構築します
2. **コンパイラ** - ASTを命令列に変換します
3. **マッチャー** - 命令列を実行して入力テキストとマッチングを行います

これらのコンポーネントは分離されており、正規表現エンジンの各部分を独立して変更できます。

## 注意事項

- バックトラック型の正規表現エンジンは悪意のある入力に対して指数関数的な時間を要する可能性があります（「巨大な」バックトラックに対して脆弱）
- このライブラリは学習目的で作成されており、パフォーマンスが必要な本番環境での使用は推奨されません
- 完全に互換性のある標準のGoのregexpパッケージの代替品ではありません

## ライセンス

このプロジェクトはMITライセンスの下で公開されています。詳細については[LICENSE](LICENSE)ファイルを参照してください。
