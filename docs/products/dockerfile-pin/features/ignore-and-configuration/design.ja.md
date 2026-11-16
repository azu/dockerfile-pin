# 除外と設定の設計

[English](design.md)

## この文書について

この設計は、[dockerfile-pin](../../product.ja.md)で[除外と設定](requirements.ja.md)をどう実装しているかを説明します。

### 想定読者

除外パターンや設定ファイルを変える人とエージェント。

### 前提知識

doublestarのglob構文。

### この文書で扱わないこと

`run`と`check`が結果をどう使うか。

## 実装方針

`cmd/ignore.go`に4つの小さな関数があり、`run`と`check`が開始時に同じ順序で呼びます。

`LoadConfig`は、現在のディレクトリで`.dockerfile-pin.yaml`、`.dockerfile-pin.yml`の順に試し、最初に存在したものを`Config`に変換します。どちらもなければ空の`Config`を返します。
`MergeIgnorePatterns`は、ファイルのパターンの後ろにコマンドラインのパターンを足します。一致の判定は最後に一致したものが勝つので、この順序によってコマンドラインが優先されます。
`ValidatePatterns`は、先頭の`!`を外し、各パターンを空の文字列に対して試します。そのため、正しくないパターンは、ファイルを読む前にコマンドを止めます。
`IsIgnored`は、パターンを順にたどり、一致するたびに答えを「除外する」に、否定なら「除外しない」にします。最後の答えを返します。`.gitignore`と同じ考え方です。

呼び出し側は、digestを除いたイメージを渡します。`check`は除外した参照を`ignored`付きの`SKIP`として報告します。`run`は解決するイメージに入れないので、digestが付くことはありません。

globでの一致は、[#30](https://github.com/azu/dockerfile-pin/pull/30)で部分文字列の一致を置き換えたものです。このPRは設定ファイルも追加し、`run`にも`--ignore-images`を足し、フラグの1回の指定を、カンマ区切りの一覧ではなく1つのパターンにしました。

## 構成図とシーケンス図

図は必要ありません。関数を順に呼ぶだけです。

## データモデルとER図

`Config`は、`IgnoreImages []string`（`ignore-images`）と`MinAge *int`（`min-age`）を持ちます。

## APIとインターフェース

`run`と`check`の`--ignore-images <pattern>`（繰り返し可）と、設定ファイルの2つのキーです。
エラーは、`reading <file>`、`parsing <file>`、`invalid ignore pattern "<pattern>"`です。

## 外部サービスとの連携

ありません。

## データの取り扱い

設定はコマンドごとに1回読み、メモリーに持ちます。

## 認証・認可の注意点

ありません。

## セキュリティ上の考慮事項

Requirementsに書いたこと以外はありません。

## デプロイ時の注意点

ありません。

## 検討した代替手段

### 部分文字列での一致

最初の実装は、指定した文字列を含むイメージを除外していました。レジストリや組織を正確に表せず、例外を作る方法もなかったので、否定のあるglobパターンに置き換えました（[#30](https://github.com/azu/dockerfile-pin/pull/30)）。

## 未決事項

ありません。
