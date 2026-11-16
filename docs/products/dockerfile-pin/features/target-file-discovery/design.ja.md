# 対象ファイルの探索の設計

[English](design.md)

## この文書について

この設計は、[dockerfile-pin](../../product.ja.md)で[対象ファイルの探索](requirements.ja.md)をどう実装しているかを説明します。

### 想定読者

どのファイルを見つけるか、種類をどう決めるかを変える人とエージェント。

### 前提知識

globパターンと`git ls-files`。

### この文書で扱わないこと

ファイルを読むことは、イメージ参照の機能に属します。

## 実装方針

`cmd/files.go`に、`run`と`check`の両方が呼ぶ2つの関数があります。`FindFiles`はパスを返し、`DetectFileType`は1つのパスを種類に対応させます。

### ファイルを見つける

`FindFiles`は、3つの指定方法を優先順に扱います。
`-f`があれば、ファイルが存在することを確かめ、それだけを返します。
`--glob`があれば、`doublestar.FilepathGlob`でパターンを展開し、何にも一致しなければ失敗します。
どちらもなければ、`git ls-files --cached --others --exclude-standard`を実行し、既定のパターンに一致する行を残します。`.gitignore`に従えるのはこのためです。
そのコマンドが失敗するか何も返さないときは、同じパターンで現在のディレクトリをたどり、`node_modules`か`vendor`を含むパスを落とします。

既定の探索は、gitとファイルシステムのglobの間で行き来してきました。決め打ちの除外リストを`git ls-files`が置き換え、その後[#19](https://github.com/azu/dockerfile-pin/pull/19)がgitリポジトリへの依存をなくすことを提案しました。今のコードは両方を持ち、gitを先に、globを代わりの手段として使います。

### 種類を決める

`DetectFileType`は、パスの区切りをスラッシュにそろえ、ファイル名を小文字にして、Requirementsに挙げた順で規則を当てはめます。
composeの名前は、GitLabのコンポーネントと`.gitlab/ci`の配置より先に調べます。これらの配置は、ほかのツールも使うディレクトリ名を手がかりにしているためで、`templates/docker-compose.yml`はcomposeファイルのままです。
コンポーネントの配置は、整えたパスに対する`doublestar.Match`で、ルートから一致させます。GitLabがコンポーネントを公開するのは、リポジトリのルートにある`templates`ディレクトリからだけのためです。
既定のパターンに入れているのは`.gitlab-ci.yml`だけです。`templates`はありふれたディレクトリ名で、`.gitlab/ci`はGitLabが定めていない慣習のためです（[#49](https://github.com/azu/dockerfile-pin/pull/49)）。

## 構成図とシーケンス図

図は必要ありません。2つの純粋な関数を順に呼ぶだけです。

## データモデルとER図

`FileType`は列挙で、`FileTypeDockerfile`、`FileTypeCompose`、`FileTypeActions`、`FileTypeGitLab`です。

## APIとインターフェース

`FindFiles(filePath, globPattern string) ([]string, error)`と`DetectFileType(path string) FileType`です。
エラーは、`file not found: <path>`、`invalid glob pattern`、`no files matched pattern: <pattern>`と、既定の探索で何も見つからないときのメッセージです。

## 外部サービスとの連携

ありません。`git`はローカルのコマンドとして実行し、なくてもエラーにしません。

## データの取り扱い

パスはメモリーに持つだけです。

## 認証・認可の注意点

ありません。

## セキュリティ上の考慮事項

Requirementsに書いたこと以外はありません。

## デプロイ時の注意点

ありません。

## 検討した代替手段

### gitを使わずglobだけにする

gitリポジトリへの依存をなくすために[#19](https://github.com/azu/dockerfile-pin/pull/19)で提案されましたが、`.gitignore`に従いません。今のコードはglobを代わりの手段としてだけ使います。

### 内容から種類を決める

採用しません。これらのファイルを置く場所は、GitHub Actionsの`.github/workflows/`のように、それぞれのプラットフォームが決めています。パスだけで種類がわかるので、すべてのファイルの内容を読んで種類を推測するのは、得るもののない無駄な探索になります。

## 未決事項

ありません。
